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
	"strings"
)

// For storing a single type, from the Accept header, split into its various
// pieces.
type AcceptType struct {
	ContentTypes []string
	Variables    map[string]string
}

type AcceptPriority struct {
	Types    []AcceptType
	Priority float32
}

// Creates an accept type from the string representation of a single type
// in an Accept header.
func NewAcceptType(accept string) AcceptType {
	acceptType := new(AcceptType)

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

// Creates a map of accept types from a full Accept header string.  Each
// index in the resulting map is priority level, and the each value is a
// slice of all types requested at that priority level.
func ParseAcceptTypes(accept string) map[float32][]AcceptType {
	acceptPieces := strings.Split(accept, ",")
	types := make(map[float32][]AcceptType)
	for _, acceptStr := range acceptPieces {
		acceptType := NewAcceptType(acceptStr)
		types[acceptType.Priority] = append(types[acceptType.Priority], acceptType)
	}
	return types
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

	for _, codec := range s.codecs {
		if strings.Contains(strings.ToLower(accept), strings.ToLower(codec.ContentType())) {
			return codec, nil
		} else if strings.ToLower(codec.FileExtension()) == strings.ToLower(extension) {
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
