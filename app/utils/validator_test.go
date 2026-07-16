package utils

import "testing"

func TestDetectLoginIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want LoginIdentifierKind
	}{
		{"+8613800138000", LoginIdentifierPhone},
		{"+14155552671", LoginIdentifierPhone},
		{"user@example.com", LoginIdentifierEmail},
		{"alice", LoginIdentifierUsername},
		{"13800138000", LoginIdentifierUsername}, // 裸号无 "+"，按用户名处理（登录走区号选择器发 E.164）
	}
	for _, c := range cases {
		if got := DetectLoginIdentifier(c.in); got != c.want {
			t.Errorf("DetectLoginIdentifier(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizePhoneToE164(t *testing.T) {
	// 裸号按默认地区 CN 规范化
	got, err := NormalizePhoneToE164("13800138000", DefaultPhoneRegion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "+8613800138000" {
		t.Errorf("got %q, want +8613800138000", got)
	}

	// 已带国家码的国际号
	got, err = NormalizePhoneToE164("+14155552671", DefaultPhoneRegion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "+14155552671" {
		t.Errorf("got %q, want +14155552671", got)
	}

	// 非法号码报错
	if _, err := NormalizePhoneToE164("123", DefaultPhoneRegion); err == nil {
		t.Error("expected error for invalid phone, got nil")
	}
}

func TestValidateUsernameFormat(t *testing.T) {
	valid := []string{"alice", "user_01", "Bob123"}
	for _, u := range valid {
		if err := ValidateUsernameFormat(u); err != nil {
			t.Errorf("ValidateUsernameFormat(%q) unexpected error: %v", u, err)
		}
	}
	invalid := []string{"12345678", "ab", "user@x", "with space"}
	for _, u := range invalid {
		if err := ValidateUsernameFormat(u); err == nil {
			t.Errorf("ValidateUsernameFormat(%q) expected error, got nil", u)
		}
	}
}
