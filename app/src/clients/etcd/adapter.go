package etcd

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/zap/zapcore"
)

// zapCoreBridge bridges zapcore.Core to zerolog
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

func (b *zapCoreBridge) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if b.Enabled(entry.Level) {
		return checked.AddCore(entry, b)
	}
	return checked
}

func (b *zapCoreBridge) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	var event *zerolog.Event
	switch entry.Level {
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

func (b *zapCoreBridge) Sync() error { return nil }

type zerologLike[T any] interface {
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
	Interface(key string, v interface{}) T
	Err(err error) T
}

func addZapField[T any, Z zerologLike[T]](z Z, f *zapcore.Field) T {
	switch f.Type {

	// ─── Basic primitives ────────────────────────────────────────────────
	case zapcore.BoolType:
		return z.Bool(f.Key, f.Integer != 0)

	case zapcore.StringType:
		return z.Str(f.Key, f.String)

	case zapcore.ByteStringType:
		return z.Str(f.Key, string(f.Interface.([]byte)))

	case zapcore.Int64Type:
		return z.Int64(f.Key, f.Integer)

	case zapcore.Int32Type:
		return z.Int32(f.Key, int32(f.Integer))

	case zapcore.Int16Type:
		return z.Int16(f.Key, int16(f.Integer))

	case zapcore.Int8Type:
		return z.Int8(f.Key, int8(f.Integer))

	case zapcore.Uint64Type:
		return z.Uint64(f.Key, uint64(f.Integer))

	case zapcore.Uint32Type:
		return z.Uint32(f.Key, uint32(f.Integer))

	case zapcore.Uint16Type:
		return z.Uint16(f.Key, uint16(f.Integer))

	case zapcore.Uint8Type:
		return z.Uint8(f.Key, uint8(f.Integer))

	case zapcore.UintptrType:
		return z.Uint64(f.Key, uint64(f.Integer))

	// ─── Floating point and complex numbers ──────────────────────────────
	case zapcore.Float64Type:
		return z.Float64(f.Key, math.Float64frombits(uint64(f.Integer)))

	case zapcore.Float32Type:
		return z.Float32(f.Key, math.Float32frombits(uint32(f.Integer)))

	case zapcore.Complex128Type:
		return z.Str(f.Key, fmt.Sprintf("%v", f.Interface.(complex128)))

	case zapcore.Complex64Type:
		return z.Str(f.Key, fmt.Sprintf("%v", f.Interface.(complex64)))

	// ─── Durations and times ─────────────────────────────────────────────
	case zapcore.DurationType:
		return z.Dur(f.Key, time.Duration(f.Integer))

	case zapcore.TimeType:
		t := time.Unix(0, f.Integer)
		if loc, ok := f.Interface.(*time.Location); ok {
			t = t.In(loc)
		}
		return z.Time(f.Key, t)

	case zapcore.TimeFullType:
		if t, ok := f.Interface.(time.Time); ok {
			return z.Time(f.Key, t)
		}
		return z.Interface(f.Key, f.Interface)

	// ─── Binary / reflection / stringers ─────────────────────────────────
	case zapcore.BinaryType:
		if b, ok := f.Interface.([]byte); ok {
			return z.Bytes(f.Key, b)
		}
		return z.Interface(f.Key, f.Interface)

	case zapcore.ReflectType:
		return z.Interface(f.Key, f.Interface)

	case zapcore.StringerType:
		if s, ok := f.Interface.(fmt.Stringer); ok {
			return z.Str(f.Key, s.String())
		}
		return z.Interface(f.Key, f.Interface)

	case zapcore.ErrorType:
		if err, ok := f.Interface.(error); ok {
			return z.Err(err)
		}
		return z.Interface(f.Key, f.Interface)

	// ─── Object and array marshalers ─────────────────────────────────────
	case zapcore.ObjectMarshalerType, zapcore.InlineMarshalerType:
		if om, ok := f.Interface.(zapcore.ObjectMarshaler); ok {
			adapter := zapObjectMarshalerAdapter{om}
			return z.Object(f.Key, &adapter)
		}
		return z.Interface(f.Key, f.Interface)

	case zapcore.ArrayMarshalerType:
		if am, ok := f.Interface.(zapcore.ArrayMarshaler); ok {
			adapter := zapArrayMarshalerAdapter{am}
			return z.Array(f.Key, &adapter)
		}
		return z.Interface(f.Key, f.Interface)

	// ─── Namespace handling ──────────────────────────────────────────────
	case zapcore.NamespaceType:
		// zerolog uses nested loggers for namespaces
		return z.Str(f.Key, "")

	// ─── Skip / unknown ──────────────────────────────────────────────────
	case zapcore.SkipType, zapcore.UnknownType:
		return z.Str(f.Key, "unknown_type")

	default:
		// Safe fallback
		return z.Interface(f.Key, f.Interface)
	}
}

// ─────────────────────────────────────────────────────────────
// zapObjectMarshalerAdapter — converts zap ObjectMarshaler → zerolog object
// ─────────────────────────────────────────────────────────────
type zapObjectMarshalerAdapter struct {
	m zapcore.ObjectMarshaler
}

func (a *zapObjectMarshalerAdapter) MarshalZerologObject(e *zerolog.Event) {
	if a.m == nil {
		return
	}

	// Marshal into zap's built-in map encoder
	mapEnc := zapcore.NewMapObjectEncoder()
	if err := a.m.MarshalLogObject(mapEnc); err != nil {
		e.Err(err)
		return
	}

	// Dump all captured fields into Zerolog
	for k, v := range mapEnc.Fields {
		e.Interface(k, v)
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
	_ = a.m.MarshalLogArray(&encoder)
}

/***************  ArrayEncoder bridge  ****************/

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

func (z *zerologArrayEncoder) AppendReflected(v interface{}) error {
	z.ae.Interface(v)
	return nil
}
