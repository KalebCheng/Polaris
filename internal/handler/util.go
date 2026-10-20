// Package handler 提供 HTTP 处理器：绑定参数、调用服务、渲染统一响应。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// bindJSON 绑定并校验 JSON 请求体。
func bindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		bizerrors.FailInvalidParams(c, err)
		return false
	}
	return true
}

// pageParams 解析分页参数（page/size），并返回 offset/limit/page/size。
func pageParams(c *gin.Context) (offset, limit, page, size int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("size", "20"))
	offset, limit = service.Page(page, size)
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	return offset, limit, page, size
}

// pathParam 读取路径参数。
func pathParam(c *gin.Context, name string) string { return c.Param(name) }

// pathID 读取路径参数中的资源主键 id。
// 约定：对外 HTTP 路径的资源标识**统一用数字主键**（不再用 name/code），
// 避免"标识改名就断链"；非数字或非正数返回 false，由调用方按"未找到"处理。
func pathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(pathParam(c, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// query 读取查询参数。
func query(c *gin.Context, name string) string { return c.Query(name) }

// queryIDValue 读取查询参数中的正整数主键；缺失或非法返回 0（= 不过滤）。
// 与 pathID 的区别：过滤条件里的 0 就是"全部"，不需要区分"未提供"与"非法"。
func queryIDValue(c *gin.Context, name string) int64 {
	id, err := strconv.ParseInt(c.Query(name), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// pathIDValue 读取路径参数 id（解析失败返回 0）。
// 说明：资源路径统一用主键 id，服务层按 id 语义访问；组织资源（institutions）直接吃这个 id。
func pathIDValue(c *gin.Context) int64 {
	id, _ := pathID(c, "id")
	return id
}
