package bson

import (
	"encoding"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
)

var DefaultStructCodec *bsoncodec.StructCodec
var DefaultRegistry *bsoncodec.Registry

var tMarshaler = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()

func init() {
	var err error

	DefaultStructCodec, err = bsoncodec.NewStructCodec(bsoncodec.JSONFallbackStructTagParser)
	if err != nil {
		panic(err)
	}

	DefaultRegistry = bson.NewRegistryBuilder().
		RegisterHookEncoder(tMarshaler,
			bsoncodec.ValueEncoderFunc(TextMarshalerEncodeValue),
		).
		RegisterDefaultEncoder(reflect.Struct, DefaultStructCodec).
		Build()
}

func Marshal(v interface{}) ([]byte, error) {
	return bson.MarshalExtJSONWithRegistry(DefaultRegistry, v, false, false)
}

// MarshalerEncodeValue is the ValueEncoderFunc for TextMarshaler implementations.
func TextMarshalerEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	// Either val or a pointer to val must implement TextMarshaler
	switch {
	case !val.IsValid():
		return bsoncodec.ValueEncoderError{Name: "TextMarshalerEncodeValue", Types: []reflect.Type{tMarshaler}, Received: val}
	case val.Type().Implements(tMarshaler):
		// If Marshaler is implemented on a concrete type, make sure that val isn't a nil pointer
		if isImplementationNil(val, tMarshaler) {
			return vw.WriteNull()
		}
	case reflect.PtrTo(val.Type()).Implements(tMarshaler) && val.CanAddr():
		val = val.Addr()
	default:
		return bsoncodec.ValueEncoderError{Name: "TextMarshalerEncodeValue", Types: []reflect.Type{tMarshaler}, Received: val}
	}

	fn := val.Convert(tMarshaler).MethodByName("MarshalText")
	returns := fn.Call(nil)

	if !returns[1].IsNil() {
		return returns[1].Interface().(error)
	}
	data := returns[0].Interface().([]byte)
	return vw.WriteString(string(data))
}

// isImplementationNil returns if val is a nil pointer and inter is implemented on a concrete type
func isImplementationNil(val reflect.Value, inter reflect.Type) bool {
	vt := val.Type()
	for vt.Kind() == reflect.Ptr {
		vt = vt.Elem()
	}
	return vt.Implements(inter) && val.Kind() == reflect.Ptr && val.IsNil()
}
