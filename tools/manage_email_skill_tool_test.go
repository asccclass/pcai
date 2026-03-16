package tools

import "testing"

func TestNormalizeManageEmailSkillArgsSearch(t *testing.T) {
	action, args, err := normalizeManageEmailSkillArgs(ManageEmailSkillArgs{Query: "is:unread", Limit: "5"})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if action != "search" {
		t.Fatalf("expected search action, got %q", action)
	}
	if args != "\"is:unread\" --max 5" {
		t.Fatalf("unexpected args: %q", args)
	}
}

func TestNormalizeManageEmailSkillArgsGet(t *testing.T) {
	action, args, err := normalizeManageEmailSkillArgs(ManageEmailSkillArgs{Action: "get", MessageID: "msg123"})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if action != "get" || args != "msg123" {
		t.Fatalf("unexpected result: action=%q args=%q", action, args)
	}
}

func TestNormalizeManageEmailSkillArgsThreadGet(t *testing.T) {
	action, args, err := normalizeManageEmailSkillArgs(ManageEmailSkillArgs{ThreadID: "thread456"})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if action != "thread get" || args != "thread456" {
		t.Fatalf("unexpected result: action=%q args=%q", action, args)
	}
}

func TestNormalizeManageEmailSkillArgsRejectsBadLimit(t *testing.T) {
	_, _, err := normalizeManageEmailSkillArgs(ManageEmailSkillArgs{Query: "is:unread", Limit: "abc"})
	if err == nil {
		t.Fatal("expected invalid limit error")
	}
}
