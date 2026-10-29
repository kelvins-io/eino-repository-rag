package handler

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/service"
)

func TestClassifyHandlerError(t *testing.T) {
	tests := []struct {
		err  error
		code int
	}{
		{service.ErrForbidden, http.StatusForbidden},
		{fmt.Errorf("知识库不存在"), http.StatusNotFound},
		{fmt.Errorf("文档正在索引中，请稍后再删除"), http.StatusConflict},
		{fmt.Errorf("name is required"), http.StatusBadRequest},
		{fmt.Errorf("无效的文档 ID"), http.StatusBadRequest},
		{fmt.Errorf("ids 不能为空"), http.StatusBadRequest},
		{fmt.Errorf("缺少上传文件"), http.StatusBadRequest},
		{fmt.Errorf("缺少文档链接"), http.StatusBadRequest},
		{fmt.Errorf("单次导入的文档链接不能超过 20 个，当前 21 个"), http.StatusBadRequest},
		{fmt.Errorf("第 1 个文档链接格式无效，路径需要以支持的文档扩展名结尾（可以带查询参数）"), http.StatusBadRequest},
		{fmt.Errorf("第 2 个文档类型无效（.exe）"), http.StatusBadRequest},
		{fmt.Errorf("第 2 个文档链接与第 1 个重复，不能重复提交"), http.StatusBadRequest},
		{fmt.Errorf("directory 不属于指定知识库"), http.StatusBadRequest},
		{fmt.Errorf("知识库下仍有 2 个文档，无法删除"), http.StatusBadRequest},
		{fmt.Errorf("parse file: empty"), http.StatusInternalServerError},
		{fmt.Errorf("导入文件 %q 失败: %w", "a.pdf", errors.New("save upload: permission denied")), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		code, msg := classifyHandlerError(tt.err)
		if code != tt.code {
			t.Fatalf("err=%v code=%d want=%d msg=%s", tt.err, code, tt.code, msg)
		}
		if msg == "" {
			t.Fatalf("empty message for %v", tt.err)
		}
	}
}
