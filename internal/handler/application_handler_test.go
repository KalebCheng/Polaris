package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"polaris/internal/repository"
	"polaris/internal/service"
)

// fakeApplicationRepo 最小内存实现（只覆盖本用例需要的读写）。
type fakeApplicationRepo struct {
	items map[int64]*repository.Application
}

func (r *fakeApplicationRepo) Create(_ context.Context, app *repository.Application) (int64, error) {
	app.ID = int64(len(r.items) + 1)
	clone := *app
	r.items[app.ID] = &clone
	return app.ID, nil
}

func (r *fakeApplicationRepo) GetByID(_ context.Context, id int64) (*repository.Application, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, errHandlerTestNotFound
	}
	clone := *item
	return &clone, nil
}

func (r *fakeApplicationRepo) GetByCode(_ context.Context, institutionID int64, code string) (*repository.Application, error) {
	for _, item := range r.items {
		if item.InstitutionID == institutionID && item.Code == code {
			clone := *item
			return &clone, nil
		}
	}
	return nil, errHandlerTestNotFound
}

func (r *fakeApplicationRepo) GetByClientID(_ context.Context, clientID string) (*repository.Application, error) {
	for _, item := range r.items {
		if item.ClientID == clientID {
			clone := *item
			return &clone, nil
		}
	}
	return nil, errHandlerTestNotFound
}

func (r *fakeApplicationRepo) Update(_ context.Context, app *repository.Application) error {
	if _, ok := r.items[app.ID]; !ok {
		return errHandlerTestNotFound
	}
	clone := *app
	r.items[app.ID] = &clone
	return nil
}

func (r *fakeApplicationRepo) Delete(_ context.Context, id int64) error {
	delete(r.items, id)
	return nil
}

func (r *fakeApplicationRepo) List(context.Context, repository.ListQuery) ([]*repository.Application, error) {
	return nil, nil
}

func (r *fakeApplicationRepo) Count(context.Context, repository.ListQuery) (int64, error) {
	return 0, nil
}

type handlerTestNotFound struct{}

func (handlerTestNotFound) Error() string { return "not found" }

var errHandlerTestNotFound = handlerTestNotFound{}

// 回归：应用是"标识(code)与名称(name)分离"的资源，PUT 必须把路径 id 解析出的**标识**写回 Code，
// 而不是 Name。
//
// 曾写成 `app.Name = codeByID(c)`（当时 name 还是标识），后果：
//  1. 每次保存都把用户填的「名称」覆盖成标识（"Net Bird" → "net-bird"）；
//  2. 只带 name 的部分更新因 Code 为空而 404。
func newApplicationHandlerForTest(row *repository.Application) (*ApplicationHandler, *fakeApplicationRepo) {
	gin.SetMode(gin.TestMode)
	repo := &fakeApplicationRepo{items: map[int64]*repository.Application{row.ID: row}}
	svc := service.NewApplicationService(repo, nil, service.NewAuditRecorder(nil))
	// 访问授权服务一并装配（列表处理器会给受限应用补"授权主体数"）；
	// 本用例的应用非受限，因此不会触达主体仓库。
	access := service.NewApplicationAccessService(nil, nil, nil, nil, nil, nil, service.NewAuditRecorder(nil))
	return NewApplicationHandler(svc, access), repo
}

func putApplication(h *ApplicationHandler, id, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Request = httptest.NewRequest("PUT", "/api/v1/applications/"+id+"?institution=2",
		strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Update(c)
	return c, recorder
}

// 回归：应用是"标识(code)与名称(name)分离"的资源，PUT 必须把路径 id 解析出的**标识**写回 Code，
// 而不是 Name。
//
// 曾写成 `app.Name = codeByID(c)`（当时 name 还是标识），后果：
//  1. 每次保存都把用户填的「名称」覆盖成标识（"Net Bird" → "net-bird"）；
//  2. 请求体不带 code 时，服务层按空标识查库 → 直接 404。
func TestApplicationUpdateKeepsSubmittedName(t *testing.T) {
	h, repo := newApplicationHandlerForTest(&repository.Application{
		ID: 69, InstitutionID: 2, Code: "net-bird", Name: "net-bird", ClientID: "cid-1",
		GrantTypes: []string{"password"}, State: "active",
	})

	c, recorder := putApplication(h, "69", `{"name":"Net Bird","grant_types":["password"],"state":"active"}`)
	if len(c.Errors) > 0 {
		t.Fatalf("保存应成功，实际错误: %v", c.Errors.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"code":"OK"`) {
		t.Fatalf("响应应为成功: %s", body)
	}
	if stored := repo.items[69]; stored.Name != "Net Bird" {
		t.Fatalf("名称被覆盖：name = %q, want %q", stored.Name, "Net Bird")
	}
	if stored := repo.items[69]; stored.Code != "net-bird" {
		t.Fatalf("标识不应被改动：code = %q", stored.Code)
	}
}

// 只带 name（不带 code）的部分更新也必须能**按路径标识定位到应用**：
// 修复前 Code 为空 → 服务层查不到 → NOT_FOUND（后续校验错误则属正常语义）。
func TestApplicationUpdateLocatesByPathVariable(t *testing.T) {
	h, _ := newApplicationHandlerForTest(&repository.Application{
		ID: 69, InstitutionID: 2, Code: "net-bird", Name: "net-bird",
		GrantTypes: []string{"password"}, State: "active",
	})

	c, _ := putApplication(h, "69", `{"name":"Net Bird"}`)
	if errors := c.Errors.String(); strings.Contains(errors, "NOT_FOUND") {
		t.Fatalf("应用应按路径标识被定位到，不应报未找到: %s", errors)
	}
}
