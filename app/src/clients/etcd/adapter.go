package etcd

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/zap/zapcore"
)

// zapCoreBridge bridges zapcore.Core to zerolog.
type zapCoreBridge struct {
	logger zerolog.Logger
}

func (b *zapCoreBridge) Enabled(level zapcore.Level) bool {
	switch level {
	case zapcore.DebugLevel:
		return b.logger.GetLevel() <= zerolog.DebugLevel
	case zapcore.InfoLevel:
		return b.logger.GetLevel() <= zerolog.InfoLevel
	case zapcore.WarnLevel:
		return b.logger.GetLevel() <= zerolog.WarnLevel
	case zapcore.ErrorLevel:
		return b.logger.GetLevel() <= zerolog.ErrorLevel
	case zapcore.FatalLevel:
		return b.logger.GetLevel() <= zerolog.FatalLevel
	case zapcore.DPanicLevel, zapcore.PanicLevel:
		return b.logger.GetLevel() <= zerolog.PanicLevel
	case zapcore.InvalidLevel:
		return b.logger.GetLevel() <= zerolog.NoLevel
	default:
		return true
	}
}

func (b *zapCoreBridge) With(fields []zapcore.Field) zapcore.Core {
	logCtx := b.logger.With()
	for _, f := range fields {
		logCtx = addZapField(logCtx, &f)
	}
	return &zapCoreBridge{logger: logCtx.Logger()}
}

func (b *zapCoreBridge) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry { //nolint:gocritic // Standard signature
	if b.Enabled(entry.Level) {
		return checked.AddCore(entry, b)
	}
	return checked
}

func (b *zapCoreBridge) Write(entry zapcore.Entry, fields []zapcore.Field) error { //nolint:gocritic // Standard signature
	var event *zerolog.Event
	switch entry.Level { //nolint:revive // Standard switch over zapcore.Level
	case zapcore.DebugLevel:
		event = b.logger.Debug()
	case zapcore.InfoLevel:
		event = b.logger.Info()
	case zapcore.WarnLevel:
		event = b.logger.Warn()
	case zapcore.ErrorLevel:
		event = b.logger.Error()
	case zapcore.FatalLevel:
		event = b.logger.Fatal()
	case zapcore.DPanicLevel, zapcore.PanicLevel:
		event = b.logger.Panic()
	case zapcore.InvalidLevel:
		event = b.logger.Info()
	default:
		event = b.logger.Info()
	}

	for _, f := range fields {
		event = addZapField(event, &f)
	}
	event = event.Str("stack", entry.Stack)
	event = event.Str("caller", entry.Caller.String())

	event.Msg(entry.Message)
	return nil
}

func (b *zapCoreBridge) Sync() error { //nolint:revive // Implement interface
	return nil
}

type zerologLike[T any] interface { //nolint:interfacebloat // Generic interface for zerolog-like loggers
	Bool(key string, val bool) T
	Int8(key string, i int8) T
	Int16(key string, i int16) T
	Int32(key string, i int32) T
	Int64(key string, val int64) T
	Uint8(key string, val uint8) T
	Uint16(key string, val uint16) T
	Uint32(key string, val uint32) T
	Uint64(key string, val uint64) T
	Float32(key string, val float32) T
	Float64(key string, val float64) T
	Bytes(key string, v []byte) T
	Str(key, val string) T
	Time(key string, t time.Time) T
	Dur(key string, d time.Duration) T
	Array(key string, arr zerolog.LogArrayMarshaler) T
	Object(key string, obj zerolog.LogObjectMarshaler) T
	Interface(key string, v any) T
	Err(err error) T
}

func addZapField[T any, Z zerologLike[T]](zevent Z, field *zapcore.Field) T {
	switch field.Type { //nolint:revive // Standard switch over zapcore.Field.Type
	// ─── Basic primitives ────────────────────────────────────────────────
	case zapcore.BoolType:
		return zevent.Bool(field.Key, field.Integer != 0)

	case zapcore.StringType:
		return zevent.Str(field.Key, field.String)

	case zapcore.ByteStringType:
		return zevent.Str(field.Key, string(field.Interface.([]byte))) //nolint:errcheck,forcetypeassert // We have type info

	case zapcore.Int64Type:
		return zevent.Int64(field.Key, field.Integer)

	case zapcore.Int32Type:
		return zevent.Int32(field.Key, int32(field.Integer)) //nolint:gosec // We have type info

	case zapcore.Int16Type:
		return zevent.Int16(field.Key, int16(field.Integer)) //nolint:gosec // We have type info

	case zapcore.Int8Type:
		return zevent.Int8(field.Key, int8(field.Integer)) //nolint:gosec // We have type info

	case zapcore.Uint64Type:
		return zevent.Uint64(field.Key, uint64(field.Integer)) //nolint:gosec // We have type info

	case zapcore.Uint32Type:
		return zevent.Uint32(field.Key, uint32(field.Integer)) //nolint:gosec // We have type info

	case zapcore.Uint16Type:
		return zevent.Uint16(field.Key, uint16(field.Integer)) //nolint:gosec // We have type info

	case zapcore.Uint8Type:
		return zevent.Uint8(field.Key, uint8(field.Integer)) //nolint:gosec // We have type info

	case zapcore.UintptrType:
		return zevent.Uint64(field.Key, uint64(field.Integer)) //nolint:gosec // We have type info

	// ─── Floating point and complex numbers ──────────────────────────────
	case zapcore.Float64Type:
		return zevent.Float64(field.Key, math.Float64frombits(uint64(field.Integer))) //nolint:gosec // We have type info

	case zapcore.Float32Type:
		return zevent.Float32(field.Key, math.Float32frombits(uint32(field.Integer))) //nolint:gosec // We have type info

	case zapcore.Complex128Type:
		return zevent.Str(field.Key, fmt.Sprintf("%v", field.Interface.(complex128))) //nolint:errcheck,forcetypeassert // We have type info

	case zapcore.Complex64Type:
		return zevent.Str(field.Key, fmt.Sprintf("%v", field.Interface.(complex64))) //nolint:errcheck,forcetypeassert // We have type info

	// ─── Durations and times ─────────────────────────────────────────────
	case zapcore.DurationType:
		return zevent.Dur(field.Key, time.Duration(field.Integer))

	case zapcore.TimeType:
		t := time.Unix(0, field.Integer)
		if loc, ok := field.Interface.(*time.Location); ok {
			t = t.In(loc)
		}
		return zevent.Time(field.Key, t)

	case zapcore.TimeFullType:
		if t, ok := field.Interface.(time.Time); ok {
			return zevent.Time(field.Key, t)
		}
		return zevent.Interface(field.Key, field.Interface)

	// ─── Binary / reflection / stringers ─────────────────────────────────
	case zapcore.BinaryType:
		if b, ok := field.Interface.([]byte); ok {
			return zevent.Bytes(field.Key, b)
		}
		return zevent.Interface(field.Key, field.Interface)

	case zapcore.ReflectType:
		return zevent.Interface(field.Key, field.Interface)

	case zapcore.StringerType:
		if s, ok := field.Interface.(fmt.Stringer); ok {
			return zevent.Str(field.Key, s.String())
		}
		return zevent.Interface(field.Key, field.Interface)

	case zapcore.ErrorType:
		if err, ok := field.Interface.(error); ok {
			return zevent.Err(err)
		}
		return zevent.Interface(field.Key, field.Interface)

	// ─── Object and array marshalers ─────────────────────────────────────
	case zapcore.ObjectMarshalerType, zapcore.InlineMarshalerType:
		if om, ok := field.Interface.(zapcore.ObjectMarshaler); ok {
			adapter := zapObjectMarshalerAdapter{om}
			return zevent.Object(field.Key, &adapter)
		}
		return zevent.Interface(field.Key, field.Interface)

	case zapcore.ArrayMarshalerType:
		if am, ok := field.Interface.(zapcore.ArrayMarshaler); ok {
			adapter := zapArrayMarshalerAdapter{am}
			return zevent.Array(field.Key, &adapter)
		}
		return zevent.Interface(field.Key, field.Interface)

	// ─── Namespace handling ──────────────────────────────────────────────
	case zapcore.NamespaceType:
		// zerolog uses nested loggers for namespaces
		return zevent.Str(field.Key, "")

	// ─── Skip / unknown ──────────────────────────────────────────────────
	case zapcore.SkipType, zapcore.UnknownType:
		return zevent.Str(field.Key, "unknown_type")

	default:
		// Safe fallback
		return zevent.Interface(field.Key, field.Interface)
	}
}

// ─────────────────────────────────────────────────────────────
// zapObjectMarshalerAdapter — converts zap ObjectMarshaler → zerolog object
// ─────────────────────────────────────────────────────────────
type zapObjectMarshalerAdapter struct {
	m zapcore.ObjectMarshaler
}

func (a *zapObjectMarshalerAdapter) MarshalZerologObject(event *zerolog.Event) {
	if a.m == nil {
		return
	}

	// Marshal into zap's built-in map encoder
	mapEnc := zapcore.NewMapObjectEncoder()
	if err := a.m.MarshalLogObject(mapEnc); err != nil {
		event.Err(err)
		return
	}

	// Dump all captured fields into Zerolog
	for k, v := range mapEnc.Fields {
		event.Interface(k, v)
	}
}

// ─────────────────────────────────────────────────────────────
// zapArrayMarshalerAdapter — converts zap ArrayMarshaler → zerolog array
// ─────────────────────────────────────────────────────────────
type zapArrayMarshalerAdapter struct {
	m zapcore.ArrayMarshaler
}

func (a *zapArrayMarshalerAdapter) MarshalZerologArray(ae *zerolog.Array) {
	if a.m == nil {
		return
	}
	// Use our ArrayEncoder that writes straight into zerolog.Array
	encoder := zerologArrayEncoder{ae}
	if err := a.m.MarshalLogArray(&encoder); err != nil {
		ae.Err(err)
	}
}

type zerologArrayEncoder struct {
	ae *zerolog.Array
}

func (z *zerologArrayEncoder) AppendBool(v bool)             { z.ae.Bool(v) }
func (z *zerologArrayEncoder) AppendInt(v int)               { z.ae.Int(v) }
func (z *zerologArrayEncoder) AppendInt64(v int64)           { z.ae.Int64(v) }
func (z *zerologArrayEncoder) AppendInt32(v int32)           { z.ae.Int32(v) }
func (z *zerologArrayEncoder) AppendInt16(v int16)           { z.ae.Int16(v) }
func (z *zerologArrayEncoder) AppendInt8(v int8)             { z.ae.Int8(v) }
func (z *zerologArrayEncoder) AppendUint(v uint)             { z.ae.Uint(v) }
func (z *zerologArrayEncoder) AppendUint64(v uint64)         { z.ae.Uint64(v) }
func (z *zerologArrayEncoder) AppendUint32(v uint32)         { z.ae.Uint32(v) }
func (z *zerologArrayEncoder) AppendUint16(v uint16)         { z.ae.Uint16(v) }
func (z *zerologArrayEncoder) AppendUint8(v uint8)           { z.ae.Uint8(v) }
func (z *zerologArrayEncoder) AppendUintptr(v uintptr)       { z.ae.Uint64(uint64(v)) }
func (z *zerologArrayEncoder) AppendFloat64(v float64)       { z.ae.Float64(v) }
func (z *zerologArrayEncoder) AppendFloat32(v float32)       { z.ae.Float32(v) }
func (z *zerologArrayEncoder) AppendComplex128(v complex128) { z.ae.Str(fmt.Sprintf("%v", v)) }
func (z *zerologArrayEncoder) AppendComplex64(v complex64)   { z.ae.Str(fmt.Sprintf("%v", v)) }

func (z *zerologArrayEncoder) AppendString(v string)     { z.ae.Str(v) }
func (z *zerologArrayEncoder) AppendByteString(v []byte) { z.ae.Bytes(v) } // or Str(string(v)) if you prefer text

func (z *zerologArrayEncoder) AppendTime(t time.Time)         { z.ae.Time(t) }
func (z *zerologArrayEncoder) AppendDuration(d time.Duration) { z.ae.Dur(d) }

func (z *zerologArrayEncoder) AppendObject(m zapcore.ObjectMarshaler) error {
	adapter := zapObjectMarshalerAdapter{m}
	z.ae.Object(&adapter)
	return nil
}

func (z *zerologArrayEncoder) AppendArray(m zapcore.ArrayMarshaler) error {
	adapter := zapArrayMarshalerAdapter{m}
	adapter.MarshalZerologArray(z.ae)
	return nil
}

func (z *zerologArrayEncoder) AppendReflected(v any) error {
	z.ae.Interface(v)
	return nil
}
