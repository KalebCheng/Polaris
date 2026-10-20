package service

import "encoding/json"

// jsonMarshal 包装标准库，便于统一替换。
func jsonMarshal(v interface{}) ([]byte, error) { return json.Marshal(v) }

// jsonUnmarshal 包装标准库，便于统一替换。
func jsonUnmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }
