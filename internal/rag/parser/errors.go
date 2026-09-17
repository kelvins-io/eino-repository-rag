package parser

import (
	"errors"
	"fmt"
)

// permanentError 表示重试也不会成功的解析失败（扫描件、不支持的格式、乱码 PDF 等）。
type permanentError struct {
	msg string
}

func (e *permanentError) Error() string { return e.msg }

func permanentf(format string, args ...any) error {
	return &permanentError{msg: fmt.Sprintf(format, args...)}
}

// IsPermanent 判断是否为不可恢复的文档解析错误。
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}
