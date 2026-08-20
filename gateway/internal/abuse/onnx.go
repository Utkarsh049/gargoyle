package abuse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// ONNXTreeEnsemble represents a parsed ONNX TreeEnsembleClassifier model.
type ONNXTreeEnsemble struct {
	NumFeatures   int
	NumClasses    int
	NumTrees      int
	NodesTreeIDs  []int64
	NodesNodeIDs  []int64
	NodesFeatIDs  []int64
	NodesValues   []float32
	NodesModes    []string
	NodesTrueIDs  []int64
	NodesFalseIDs []int64
	ClassTreeIDs  []int64
	ClassNodeIDs  []int64
	ClassIDs      []int64
	ClassWeights  []float32
	PostTransform string

	// Lookup maps for fast O(depth) tree traversal: [treeID][nodeID] -> NodeIndex
	treeNodeIndex map[int64]map[int64]int
	// Leaf class weights map: [treeID][nodeID][classID] -> weight
	leafWeights map[int64]map[int64]map[int64]float32
}

// Predict calculates class probabilities for a 6-element feature vector.
// Returns [P(clean), P(abuse)] where P(abuse) is at index 1.
func (te *ONNXTreeEnsemble) Predict(features []float32) ([]float32, error) {
	if len(features) < te.NumFeatures {
		return nil, fmt.Errorf("expected at least %d features, got %d", te.NumFeatures, len(features))
	}

	var abuseScore float64
	hasBinaryClass0Only := true
	for _, cid := range te.ClassIDs {
		if cid != 0 {
			hasBinaryClass0Only = false
			break
		}
	}

	classScores := make([]float64, te.NumClasses)

	for treeID := 0; treeID < te.NumTrees; treeID++ {
		tID := int64(treeID)
		nodesMap, ok := te.treeNodeIndex[tID]
		if !ok {
			continue
		}

		currentNodeID := int64(0)
		for {
			nodeIdx, exists := nodesMap[currentNodeID]
			if !exists {
				break
			}

			mode := "BRANCH_LEQ"
			if nodeIdx < len(te.NodesModes) {
				mode = te.NodesModes[nodeIdx]
			}

			if mode == "LEAF" {
				if treeLeaves, ok := te.leafWeights[tID]; ok {
					if weights, ok := treeLeaves[currentNodeID]; ok {
						for classID, w := range weights {
							if hasBinaryClass0Only && classID == 0 {
								abuseScore += float64(w)
							} else if int(classID) < len(classScores) {
								classScores[classID] += float64(w)
							}
						}
					}
				}
				break
			}

			// Branch condition: feature[id] <= value -> true_branch, else false_branch
			featID := te.NodesFeatIDs[nodeIdx]
			var featVal float32
			if int(featID) < len(features) {
				featVal = features[featID]
			}
			threshold := te.NodesValues[nodeIdx]

			if featVal <= threshold {
				currentNodeID = te.NodesTrueIDs[nodeIdx]
			} else {
				currentNodeID = te.NodesFalseIDs[nodeIdx]
			}
		}
	}

	probs := make([]float32, te.NumClasses)

	if hasBinaryClass0Only {
		// Binary classifier in skl2onnx: sum of leaf weights is P(abuse)
		pAbuse := float32(math.Max(0.0, math.Min(1.0, abuseScore)))
		pClean := float32(math.Max(0.0, math.Min(1.0, 1.0-float64(pAbuse))))
		probs[0] = pClean
		probs[1] = pAbuse
		return probs, nil
	}

	// Multi-class normalization
	var total float64
	for _, s := range classScores {
		total += s
	}

	if total > 0 {
		for i, s := range classScores {
			probs[i] = float32(s / total)
		}
	} else {
		for i := range probs {
			probs[i] = 1.0 / float32(te.NumClasses)
		}
	}

	return probs, nil
}

// LoadONNXTreeEnsemble parses an ONNX model file containing a TreeEnsembleClassifier.
func LoadONNXTreeEnsemble(filePath string) (*ONNXTreeEnsemble, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return ParseONNXModel(data)
}

// ParseONNXModel parses raw protobuf bytes of an ONNX ModelProto.
func ParseONNXModel(data []byte) (*ONNXTreeEnsemble, error) {
	te := &ONNXTreeEnsemble{
		NumFeatures:   6,
		NumClasses:    2,
		treeNodeIndex: make(map[int64]map[int64]int),
		leafWeights:   make(map[int64]map[int64]map[int64]float32),
	}

	r := bytes.NewReader(data)
	for {
		tag, wireType, err := readFieldTag(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if tag == 7 && wireType == 2 { // graph: GraphProto
			graphBytes, err := readBytes(r)
			if err != nil {
				return nil, err
			}
			if err := parseGraphProto(graphBytes, te); err != nil {
				return nil, err
			}
		} else {
			if err := skipField(r, wireType); err != nil {
				return nil, err
			}
		}
	}

	// Build lookup indices
	treeSet := make(map[int64]struct{})
	for i, tID := range te.NodesTreeIDs {
		treeSet[tID] = struct{}{}
		if _, ok := te.treeNodeIndex[tID]; !ok {
			te.treeNodeIndex[tID] = make(map[int64]int)
		}
		nID := te.NodesNodeIDs[i]
		te.treeNodeIndex[tID][nID] = i
	}
	te.NumTrees = len(treeSet)

	for i, tID := range te.ClassTreeIDs {
		nID := te.ClassNodeIDs[i]
		cID := te.ClassIDs[i]
		w := te.ClassWeights[i]

		if _, ok := te.leafWeights[tID]; !ok {
			te.leafWeights[tID] = make(map[int64]map[int64]float32)
		}
		if _, ok := te.leafWeights[tID][nID]; !ok {
			te.leafWeights[tID][nID] = make(map[int64]float32)
		}
		te.leafWeights[tID][nID][cID] = w
	}

	return te, nil
}

func parseGraphProto(data []byte, te *ONNXTreeEnsemble) error {
	r := bytes.NewReader(data)
	for {
		tag, wireType, err := readFieldTag(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if tag == 1 && wireType == 2 { // node: NodeProto
			nodeBytes, err := readBytes(r)
			if err != nil {
				return err
			}
			if err := parseNodeProto(nodeBytes, te); err != nil {
				return err
			}
		} else {
			if err := skipField(r, wireType); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseNodeProto(data []byte, te *ONNXTreeEnsemble) error {
	r := bytes.NewReader(data)
	var opType string
	for {
		tag, wireType, err := readFieldTag(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if tag == 4 && wireType == 2 { // op_type: string
			b, err := readBytes(r)
			if err != nil {
				return err
			}
			opType = string(b)
		} else if tag == 5 && wireType == 2 { // attribute: AttributeProto
			attrBytes, err := readBytes(r)
			if err != nil {
				return err
			}
			if err := parseAttributeProto(attrBytes, te); err != nil {
				return err
			}
		} else {
			if err := skipField(r, wireType); err != nil {
				return err
			}
		}
	}

	_ = opType
	return nil
}

func parseAttributeProto(data []byte, te *ONNXTreeEnsemble) error {
	r := bytes.NewReader(data)
	var name string
	var floats []float32
	var ints []int64
	var strs []string
	var strVal string

	for {
		tag, wireType, err := readFieldTag(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch tag {
		case 1: // name: string
			b, err := readBytes(r)
			if err != nil {
				return err
			}
			name = string(b)
		case 4: // s: bytes
			b, err := readBytes(r)
			if err != nil {
				return err
			}
			strVal = string(b)
		case 7: // floats: repeated float
			f, err := readRepeatedFloat32(r, wireType)
			if err != nil {
				return err
			}
			floats = append(floats, f...)
		case 8: // ints: repeated int64
			i, err := readRepeatedInt64(r, wireType)
			if err != nil {
				return err
			}
			ints = append(ints, i...)
		case 9: // strings: repeated bytes
			b, err := readBytes(r)
			if err != nil {
				return err
			}
			strs = append(strs, string(b))
		default:
			if err := skipField(r, wireType); err != nil {
				return err
			}
		}
	}

	switch name {
	case "nodes_values":
		te.NodesValues = floats
	case "class_weights":
		te.ClassWeights = floats
	case "nodes_treeids":
		te.NodesTreeIDs = ints
	case "nodes_nodeids":
		te.NodesNodeIDs = ints
	case "nodes_featureids":
		te.NodesFeatIDs = ints
	case "nodes_truenodeids":
		te.NodesTrueIDs = ints
	case "nodes_falsenodeids":
		te.NodesFalseIDs = ints
	case "class_treeids":
		te.ClassTreeIDs = ints
	case "class_nodeids":
		te.ClassNodeIDs = ints
	case "class_ids":
		te.ClassIDs = ints
	case "nodes_modes":
		te.NodesModes = strs
	case "post_transform":
		te.PostTransform = strVal
	}

	return nil
}

// Protobuf wire helpers
func readFieldTag(r *bytes.Reader) (int, int, error) {
	tagAndType, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, 0, err
	}
	tag := int(tagAndType >> 3)
	wireType := int(tagAndType & 0x7)
	return tag, wireType, nil
}

func readBytes(r *bytes.Reader) ([]byte, error) {
	length, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func readRepeatedInt64(r *bytes.Reader, wireType int) ([]int64, error) {
	if wireType == 2 { // packed
		packedBytes, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		pr := bytes.NewReader(packedBytes)
		var result []int64
		for pr.Len() > 0 {
			v, err := binary.ReadUvarint(pr)
			if err != nil {
				return nil, err
			}
			result = append(result, int64(v))
		}
		return result, nil
	}
	// single varint
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}
	return []int64{int64(v)}, nil
}

func readRepeatedFloat32(r *bytes.Reader, wireType int) ([]float32, error) {
	if wireType == 2 { // packed
		packedBytes, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		numFloats := len(packedBytes) / 4
		result := make([]float32, numFloats)
		for i := 0; i < numFloats; i++ {
			bits := binary.LittleEndian.Uint32(packedBytes[i*4 : (i+1)*4])
			result[i] = math.Float32frombits(bits)
		}
		return result, nil
	}
	var bits uint32
	if err := binary.Read(r, binary.LittleEndian, &bits); err != nil {
		return nil, err
	}
	return []float32{math.Float32frombits(bits)}, nil
}

func skipField(r *bytes.Reader, wireType int) error {
	switch wireType {
	case 0: // varint
		_, err := binary.ReadUvarint(r)
		return err
	case 1: // 64-bit
		var buf [8]byte
		_, err := io.ReadFull(r, buf[:])
		return err
	case 2: // length-delimited
		length, err := binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		_, err = r.Seek(int64(length), io.SeekCurrent)
		return err
	case 5: // 32-bit
		var buf [4]byte
		_, err := io.ReadFull(r, buf[:])
		return err
	default:
		return fmt.Errorf("unsupported wire type: %d", wireType)
	}
}
