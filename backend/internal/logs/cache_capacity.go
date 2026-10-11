package logs

import "reflect"

const (
	maximumCacheWeight = 64 << 20
	maximumColdLoads   = 2
)

// This is a conservative retention weight, not a measurement of Go heap or RSS.
// Count nested maps, typed structs/slices, strings and slice capacity without
// serializing payloads or changing JSON numbers. Shared references may be counted
// more than once. Cyclic/deep or unsupported data is returned but not cached.
func logCacheWeight(query Query, page Page) int64 {
	weight := cacheWeigher{remaining: maximumCacheWeight}
	if !weight.visit(reflect.ValueOf(query), 0) || !weight.visit(reflect.ValueOf(page), 0) {
		return maximumCacheWeight + 1
	}
	return maximumCacheWeight - weight.remaining
}

type cacheWeigher struct{ remaining int64 }

func (w *cacheWeigher) charge(count int, size uintptr) bool {
	if size == 0 {
		return true
	}
	if uint64(size) > uint64(w.remaining) || uint64(count) > uint64(w.remaining)/uint64(size) {
		return false
	}
	w.remaining -= int64(uint64(count) * uint64(size))
	return true
}

func (w *cacheWeigher) visit(value reflect.Value, depth int) bool {
	if !value.IsValid() {
		return true
	}
	if depth > 64 || !w.charge(1, max(value.Type().Size(), 1)) {
		return false
	}
	switch value.Kind() {
	case reflect.String:
		return w.charge(value.Len(), 1)
	case reflect.Interface, reflect.Pointer:
		return value.IsNil() || w.visit(value.Elem(), depth+1)
	case reflect.Map:
		if value.IsNil() {
			return true
		}
		// Allow for map metadata and bucket/index slack in addition to values.
		if !w.charge(1, 64) || !w.charge(value.Len(), 32) {
			return false
		}
		// JSON payload maps dominate log data. Iterate their existing interfaces
		// directly to avoid allocating reflected keys/values for every entry.
		if value.Type() == reflect.TypeFor[map[string]any]() && value.CanInterface() {
			for key, item := range value.Interface().(map[string]any) {
				if !w.charge(1, 16) || !w.charge(len(key), 1) || !w.charge(1, 16) || !w.visit(reflect.ValueOf(item), depth+2) {
					return false
				}
			}
			return true
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if !w.visit(iterator.Key(), depth+1) || !w.visit(iterator.Value(), depth+1) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && !w.charge(value.Cap(), value.Type().Elem().Size()) {
			return false
		}
		for index := 0; index < value.Len(); index++ {
			if !w.visit(value.Index(index), depth+1) {
				return false
			}
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if !w.visit(value.Field(index), depth+1) {
				return false
			}
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
	return true
}

// Call only while holding s.mu, including replacement and expiry paths.
func (s *Service) removeCached(query Query) {
	if cached, ok := s.cache[query]; ok {
		s.cacheWeight -= cached.weight
		delete(s.cache, query)
	}
}
