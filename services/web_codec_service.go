package services

import (
	"errors"
	"fmt"
	"github.com/stretchr/codecs"
	"github.com/stretchr/codecs/bson"
	"github.com/stretchr/codecs/constants"
	"github.com/stretchr/codecs/csv"
	"github.com/stretchr/codecs/json"
	"github.com/stretchr/codecs/jsonp"
	"github.com/stretchr/codecs/msgpack"
	"github.com/stretchr/codecs/xml"
	"strconv"
	"strings"
)

// For storing a single type, from the Accept header, split into its various
// pieces.
type AcceptType struct {
	ContentTypes []string
	Variables    map[string]string
	Size         int
	Priority     float32
	Left         *AcceptType
	Right        *AcceptType
}

func (acceptType *AcceptType) Append(next AcceptType) {
	switch {
	case acceptType.Priority == next.Priority:
		if acceptType.Left == nil {
			acceptType.Left = next
		} else if acceptType.Right == nil {
			acceptType.Right = next
		} else {
			// Try to keep the tree balanced
			if acceptType.Right.Size <= acceptType.Left.Size {
				acceptType.Right.Append(next)
			} else {
				acceptType.Left.Append(next)
			}
		}
	case next.Priority > acceptType.Priority:
		if acceptType.Left == nil {
			acceptType.Left = next
		} else {
			acceptType.Left.Append(next)
		}
	case next.Priority < acceptType.Priority:
		if acceptType.Right == nil {
			acceptType.Right = next
		} else {
			acceptType.Right.Append(next)
		}
	}
	acceptType.Size++
}

func (acceptType *AcceptType) ToSlice() []AcceptType {
	typeSlice := make([]AcceptType, acceptType.Size)
	leftEnd := 0
	if acceptType.Left != nil {
		leftEnd = acceptType.Left.Size
		for index, leftType := range acceptType.Left.ToSlice() {
			typeSlice[index] = leftType
		}
	}
	typeSlice[leftEnd] = acceptType
	if acceptType.Right != nil {
		rightStart := leftEnd + 1
		for index, rightType := range acceptType.Right.ToSlice() {
			typeSlice[rightStart + index] = rightType
		}
	}
	return typeSlice
}

func (acceptType *AcceptType) FindCodec(codecs []codecs.Codec) codecs.Codec {
	for _, codec := range codecs {
		for _, typeString := range acceptType.ContentTypes {
			if strings.ToLower(typeString) == strings.ToLower(codec.ContentType()) {
				return codec
			}
		}
	}
	return nil
}

// Creates an accept type from the string representation of a single type
// in an Accept header.
func NewAcceptType(accept string) AcceptType {
	acceptType := new(AcceptType)

	acceptType.Size = 1

	typesAndVariables := strings.Split(accept, ";")
	variables := typesAndVariables[1:]
	for _, variable := range variables {
		variableParts := strings.Split(variable, "=")
		acceptType.Variables[variableParts[0]] = variableParts[1]
	}

	typesString := typesAndVariables[0]
	types := strings.Split(typesString, "+")
	// Don't care about the category
	categoryEnd := strings.LastIndex(types[0], "/")
	types[0] = types[0][categoryEnd:]
	acceptType.ContentTypes = types

	acceptType.priority = 1.0
	if priority, ok := acceptType.variables["q"]; ok {
		if requestedPriority, err := strconv.ParseFloat(priority, 32).(float32); err == nil {
			acceptType.Priority = float32(requestedPriority)
		}
	}

	return acceptType
}

// Creates a binary tree of AcceptTypes.  They will be sorted by priority,
// with higher priorities on the left.
func ParseAcceptTypes(accept string) AcceptType {
	acceptPieces := strings.Split(accept, ",")
	var root, nextType AcceptType
	for _, acceptStr := range acceptPieces {
		nextType = NewAcceptType(acceptStr)
		if root == nil {
			root = nextType
		} else {
			root.Append(nextType)
		}
	}
	return root
}

// ErrorContentTypeNotSupported is the error for when a content type is requested that is not supported by the system
var ErrorContentTypeNotSupported = errors.New("Content type is not supported.")

// DefaultCodecs represents the list of Codecs that get added automatically by
// a call to NewWebCodecService.
var DefaultCodecs = []codecs.Codec{new(json.JsonCodec), new(jsonp.JsonPCodec), new(msgpack.MsgpackCodec), new(bson.BsonCodec), new(csv.CsvCodec), new(xml.SimpleXmlCodec)}

// WebCodecService represents the default implementation for providing access to the
// currently installed web codecs.
type WebCodecService struct {
	// codecs holds the installed codecs for this service.
	codecs []codecs.Codec
}

// NewWebCodecService makes a new WebCodecService with the default codecs
// added.
func NewWebCodecService() *WebCodecService {
	s := new(WebCodecService)
	s.codecs = DefaultCodecs
	return s
}

// Codecs gets all currently installed codecs.
func (s *WebCodecService) Codecs() []codecs.Codec {
	return s.codecs
}

// AddCodec adds the specified codec to the installed codecs list.
func (s *WebCodecService) AddCodec(codec codecs.Codec) {
	s.codecs = append(s.codecs, codec)
}

func (s *WebCodecService) assertCodecs() {
	if len(s.codecs) == 0 {
		panic("codecs: No codecs are installed - use AddCodec to add some or use NewWebCodecService for default codecs.")
	}
}

// GetCodecForResponding gets the codec to use to respond based on the
// given accept string, the extension provided and whether it has a callback
// or not.
//
// As of now, if hasCallback is true, the JSONP codec will be returned.
// This may be changed if additional callback capable codecs are added.
func (s *WebCodecService) GetCodecForResponding(accept, extension string, hasCallback bool) (codecs.Codec, error) {

	// make sure we have at least one codec
	s.assertCodecs()

	// is there a callback?  If so, look for JSONP
	if hasCallback {
		for _, codec := range s.codecs {
			if codec.ContentType() == constants.ContentTypeJSONP {
				return codec, nil
			}
		}
	}

	// Prefer the accept header
	acceptTypesRoot := ParseAcceptTypes(accept)
	for _, acceptType := range acceptTypesRoot.ToSlice() {
		if codec := acceptType.FindCodec(s.codecs); codec != nil {
			return codec, nil
		}
	}

	for _, codec := range s.codecs {
		if strings.ToLower(codec.FileExtension()) == strings.ToLower(extension) {
			return codec, nil
		} else if hasCallback && codec.CanMarshalWithCallback() {
			return codec, nil
		}
	}

	// return the first installed codec by default
	return s.codecs[0], nil
}

// GetCodec gets the codec to use to interpret the request based on the
// content type.
func (s *WebCodecService) GetCodec(contentType string) (codecs.Codec, error) {

	// make sure we have at least one codec
	s.assertCodecs()

	for _, codec := range s.codecs {

		// default codec
		if len(contentType) == 0 && codec.ContentType() == constants.ContentTypeJSON {
			return codec, nil
		}

		// match the content type
		if strings.Contains(strings.ToLower(contentType), strings.ToLower(codec.ContentType())) {
			return codec, nil
		}

	}

	return nil, errors.New(fmt.Sprintf("Content type \"%s\" is not supported.", contentType))

}

// MarshalWithCodec marshals the specified object with the specified codec and options.
// If the object implements the Facade interface, the PublicData object should be
// marshalled instead.
func (s *WebCodecService) MarshalWithCodec(codec codecs.Codec, object interface{}, options map[string]interface{}) ([]byte, error) {

	// make sure we have at least one codec
	s.assertCodecs()

	// get the public data
	publicData, err := codecs.PublicData(object, options)

	// if there was an error - return it
	if err != nil {
		return nil, err
	}

	// let the codec do its work
	return codec.Marshal(publicData, options)
}

// UnmarshalWithCodec unmarshals the specified data into the object with the specified codec.
func (s *WebCodecService) UnmarshalWithCodec(codec codecs.Codec, data []byte, object interface{}) error {

	// make sure we have at least one codec
	s.assertCodecs()

	return codec.Unmarshal(data, object)
}
