package service

import (
	"context"
	"testing"

	"polaris/internal/repository"
)

// stubDeptRepo 仅实现测试所需方法（其余经嵌入接口置为 nil）。
type stubDeptRepo struct {
	repository.DepartmentRepository
	items   []*repository.Department
	movedTo map[string]int
}

func (r *stubDeptRepo) List(context.Context, repository.ListQuery) ([]*repository.Department, error) {
	return r.items, nil
}

func (r *stubDeptRepo) GetByName(_ context.Context, institutionID int64, name string) (*repository.Department, error) {
	for _, d := range r.items {
		if d.InstitutionID == institutionID && d.Name == name {
			return d, nil
		}
	}
	return nil, errStubNotFound
}

func (r *stubDeptRepo) SetSortIndex(_ context.Context, institutionID int64, name string, sortIndex int) error {
	if r.movedTo == nil {
		r.movedTo = map[string]int{}
	}
	r.movedTo[name] = sortIndex
	for _, d := range r.items {
		if d.InstitutionID == institutionID && d.Name == name {
			d.SortIndex = sortIndex
		}
	}
	return nil
}

func dept(id int64, name string, sort int) *repository.Department {
	return &repository.Department{ID: id, InstitutionID: 1, Name: name, Level: 0, SortIndex: sort}
}

// 回归：同级下移交换两节点的 sort_index（b 与 c 交换）。
func TestDepartmentReorderDown(t *testing.T) {
	repo := &stubDeptRepo{items: []*repository.Department{
		dept(1, "a", 0), dept(2, "b", 1), dept(3, "c", 2),
	}}
	svc := &DepartmentService{depts: repo, audits: &AuditRecorder{}}
	if err := svc.Reorder(context.Background(), "admin", 1, "b", "down"); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	// b 应落到序号 2，c 落到 1
	got := map[string]int{}
	for _, d := range repo.items {
		got[d.Name] = d.SortIndex
	}
	if got["b"] != 2 || got["c"] != 1 {
		t.Fatalf("下移后 sort_index 错误: %v", got)
	}
}

// 回归：首节点上移为无操作（不报错、顺序不变）。
func TestDepartmentReorderUpAtTop(t *testing.T) {
	repo := &stubDeptRepo{items: []*repository.Department{dept(1, "a", 0), dept(2, "b", 1)}}
	svc := &DepartmentService{depts: repo, audits: &AuditRecorder{}}
	if err := svc.Reorder(context.Background(), "admin", 1, "a", "up"); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if len(repo.movedTo) != 0 {
		t.Fatalf("首节点上移应无操作，实际写入 %v", repo.movedTo)
	}
}

// 回归：非法 direction 报错。
func TestDepartmentReorderInvalidDirection(t *testing.T) {
	repo := &stubDeptRepo{items: []*repository.Department{dept(1, "a", 0), dept(2, "b", 1)}}
	svc := &DepartmentService{depts: repo, audits: &AuditRecorder{}}
	if err := svc.Reorder(context.Background(), "admin", 1, "a", "sideways"); err == nil {
		t.Fatal("非法 direction 应报错")
	}
}
