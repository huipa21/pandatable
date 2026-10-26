package admin

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"pandatable/internal/acl"
)

func (f *fixture) workspace(name string) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO meta.workspaces(name) VALUES ($1) RETURNING id::text", name).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	f.workspaces = append(f.workspaces, id)
	f.t.Cleanup(func() {
		f.exec(context.Background(), "DELETE FROM meta.grants WHERE resource_type='workspace' AND resource_id=$1", id)
	})
	return id
}

func TestWorkspacesPagingOwnersOrphansTrashAndIsolation(t *testing.T) {
	f := newFixture(t)
	alice, _ := f.account("user", "active", false)
	disabled, _ := f.account("user", "disabled", false)
	group := fixtureGroup(t, f, "owner-group")
	f.exec(f.ctx, "UPDATE meta.users SET display_name='Alice' WHERE id=$1", alice)
	first, second, trash := f.workspace("Inventory %_"), f.workspace("Inventory %_"), f.workspace("Trashed")
	f.exec(f.ctx, "UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1", trash)
	f.exec(f.ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'user',$2,'owner'),('workspace',$1,'group',$3,'owner'),
		('workspace',$4,'user',$5,'owner'),('workspace',$4,'group',$3,'owner')`,
		first, alice, group.ID, second, disabled)
	f.exec(f.ctx, "INSERT INTO meta.group_members(group_id,user_id) VALUES ($1,$2)", group.ID, alice)
	input := ListInput{Q: " INVENTORY %_ ", Limit: 1}
	list, err := f.service.Workspaces(f.ctx, f.token, input)
	ids := []string{first, second}
	slices.Sort(ids)
	if err != nil || list.Total != 2 || len(list.Items) != 1 || list.Items[0].ID != ids[0] {
		t.Fatalf("first page=%+v err=%v", list, err)
	}
	input.Offset = 1
	next, err := f.service.Workspaces(f.ctx, f.token, input)
	if err != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].ID != ids[1] {
		t.Fatalf("second page=%+v %v", next, err)
	}
	input.Offset = 2
	empty, err := f.service.Workspaces(f.ctx, f.token, input)
	if err != nil || empty.Total != 2 || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("out of range=%+v %v", empty, err)
	}
	all, err := f.service.Workspaces(f.ctx, f.token, ListInput{Limit: 500})
	if err != nil || all.Total != 2 {
		t.Fatalf("live count=%+v %v", all, err)
	}
	for _, item := range all.Items {
		if item.Orphaned != (item.ID == second) || item.OwnerTotal != 2 || len(item.Owners) != 2 || item.AttachmentBytes != 0 {
			t.Fatalf("owner rules: %+v", item)
		}
		foundDisabled := false
		for _, owner := range item.Owners {
			if owner.Type == "group" && (owner.Username != nil || owner.Status != nil) {
				t.Fatal("group owner contains user identity")
			}
			if owner.ID == disabled {
				foundDisabled = owner.Status != nil && *owner.Status == "disabled"
			}
		}
		if item.ID == second && !foundDisabled {
			t.Fatal("disabled owner must be visible")
		}
	}
	resource, err := acl.ParseResourceRef("workspace:" + first)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.resolver.Access(f.ctx, f.admin, resource)
	if err != nil || before.Visible || before.Level != acl.None {
		t.Fatalf("admin received data access: %+v %v", before, err)
	}
	ownerAccess, err := f.resolver.Access(f.ctx, alice, resource)
	if err != nil || ownerAccess.Level != acl.Owner {
		t.Fatalf("actual owner denied: %+v %v", ownerAccess, err)
	}
	f.exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", alice)
	orphan, err := f.service.Workspaces(f.ctx, f.token, ListInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range orphan.Items {
		if !item.Orphaned {
			t.Fatal("disabled and group owners cannot prevent orphan status")
		}
	}
	after, err := f.resolver.Access(f.ctx, f.admin, resource)
	if err != nil || after != before {
		t.Fatal("read-only listing changed ACL", err)
	}
	payload, _ := json.Marshal(all)
	for _, secret := range []string{password, f.phc, f.token, "email", "pg_schema", "storage_path"} {
		if strings.Contains(string(payload), secret) {
			t.Fatal("inventory leaked private data")
		}
	}
	for _, bad := range []ListInput{{Limit: 0}, {Limit: 501}, {Limit: 1, Offset: -1}, {Limit: 1, Q: "\x00"},
		{Limit: 1, Q: strings.Repeat("x", 255)}, {Limit: 1, Status: "active"}} {
		_, err := f.service.Workspaces(f.ctx, f.token, bad)
		code(t, err, "ERR_VALIDATION")
	}
}

func TestWorkspacesAttachmentMetadataAndBoundedOwners(t *testing.T) {
	f := newFixture(t)
	workspace := f.workspace("size-and-owner-bound")
	var book, table, field string
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.books(workspace_id,name,pg_schema,position,deleted_at)
		VALUES ($1,'retained','b_sizefixture',0,now()) RETURNING id::text`, workspace).Scan(&book); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.tables(book_id,name,pg_name,position,deleted_at)
		VALUES ($1,'retained','t_sizefixture',0,now()) RETURNING id::text`, book).Scan(&table); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.fields(table_id,name,ui_type,position,deleted_at)
		VALUES ($1,'file','attachment',0,now()) RETURNING id::text`, table).Scan(&field); err != nil {
		t.Fatal(err)
	}
	f.exec(f.ctx, `INSERT INTO meta.attachments(table_id,field_id,file_name,mime_type,size_bytes,sha256,storage_path)
		VALUES ($1,$2,'private-name','application/octet-stream',4096,decode(repeat('00',32),'hex'),'../never-read'),
		($1,$2,'another','application/octet-stream',17,decode(repeat('00',32),'hex'),'missing')`, table, field)
	f.exec(f.ctx, `WITH created AS (
		INSERT INTO meta.groups(name) SELECT 'bounded-owner-'||n FROM generate_series(1,55) n RETURNING id
	) INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
		SELECT 'workspace',$1,'group',id,'owner' FROM created`, workspace)
	t.Cleanup(func() {
		f.exec(context.Background(), "DELETE FROM meta.groups WHERE name LIKE 'bounded-owner-%'")
	})
	list, err := f.service.Workspaces(f.ctx, f.token, ListInput{Limit: 50})
	if err != nil || len(list.Items) != 1 || list.Items[0].AttachmentBytes != 4113 ||
		list.Items[0].OwnerTotal != 55 || len(list.Items[0].Owners) != 50 || !list.Items[0].Orphaned {
		t.Fatalf("bounded metadata inventory=%+v %v", list, err)
	}
}
