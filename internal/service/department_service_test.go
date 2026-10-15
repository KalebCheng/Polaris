package service

import (
	"context"
	"strconv"
	"testing"

	"polaris/internal/repository"
)

// stubUserCountRepo 只实现 Count（部门删除要确认"没有归属用户"）。
type stubUserCountRepo struct {
	repository.UserRepository
	count int64
}

func (r *stubUserCountRepo) Count(context.Context, repository.ListQuery) (int64, error) {
	return r.count, nil
}

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

func (r *stubDeptRepo) GetByID(_ context.Context, id int64) (*repository.Department, error) {
	for _, d := range r.items {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, errStubNotFound
}

func (r *stubDeptRepo) Count(_ context.Context, q repository.ListQuery) (int64, error) {
	if q.Field != "parent_id" {
		return 0, nil
	}
	var n int64
	for _, d := range r.items {
		if d.ParentID != nil && strconv.FormatInt(*d.ParentID, 10) == q.Value {
			n++
		}
	}
	return n, nil
}

func (r *stubDeptRepo) Update(_ context.Context, d *repository.Department) error {
	for index, item := range r.items {
		if item.ID == d.ID {
			r.items[index] = d
			return nil
		}
	}
	return errStubNotFound
}

func (r *stubDeptRepo) Delete(_ context.Context, institutionID int64, name string) error {
	for index, d := range r.items {
		if d.InstitutionID == institutionID && d.Name == name {
			r.items = append(r.items[:index], r.items[index+1:]...)
			return nil
		}
	}
	return errStubNotFound
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

// 回归：删掉最后一个子部门后，父部门的 has_child 必须复位。
//
// 根因：Delete 用存储的 has_child 标记判断"还有下级"，而删除路径从未刷新过它——
// 于是父部门在删完子部门后**永远删不掉**（返回 409 "still has child departments"），
// 只能靠"先移动一次子部门层级"（走 Update）顺带刷新，行为极其反直觉。
func TestDepartmentDeleteRefreshesParentHasChild(t *testing.T) {
	parentID := int64(1)
	repo := &stubDeptRepo{items: []*repository.Department{
		{ID: 1, InstitutionID: 1, Name: "tech", HasChild: true},
		{ID: 2, InstitutionID: 1, Name: "fe", ParentID: &parentID},
	}}
	svc := &DepartmentService{depts: repo, users: &stubUserCountRepo{}, audits: &AuditRecorder{}}

	if err := svc.Delete(context.Background(), "admin", 1, "fe"); err != nil {
		t.Fatalf("删除子部门: %v", err)
	}
	for _, d := range repo.items {
		if d.Name == "tech" && d.HasChild {
			t.Fatal("删掉最后一个子部门后父部门 has_child 应为 false")
		}
	}
	if err := svc.Delete(context.Background(), "admin", 1, "tech"); err != nil {
		t.Fatalf("父部门此时应可删除: %v", err)
	}
}
