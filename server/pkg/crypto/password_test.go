package crypto

import (
	"strings"
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	const pwd = "correct horse battery"
	hashed, err := HashPassword(pwd)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hashed == pwd || strings.Contains(hashed, pwd) {
		t.Fatal("哈希不应包含明文密码")
	}
	// bcrypt 哈希前缀标识
	if !strings.HasPrefix(hashed, "$2") {
		t.Fatalf("应为 bcrypt 哈希($2 开头), got %q", hashed[:2])
	}
	if !CheckPassword(hashed, pwd) {
		t.Fatal("正确密码应校验通过")
	}
	if CheckPassword(hashed, "wrong-password") {
		t.Fatal("错误密码不应通过")
	}
}

func TestHashSalted(t *testing.T) {
	// 同一密码两次哈希结果应不同（盐随机）
	h1, _ := HashPassword("samepassword123")
	h2, _ := HashPassword("samepassword123")
	if h1 == h2 {
		t.Fatal("bcrypt 两次哈希应因随机盐而不同")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	// 系统按要求不限制密码强度：任意非空密码都应通过
	if err := ValidatePasswordStrength("1"); err != nil {
		t.Fatalf("短密码应通过: %v", err)
	}
	if err := ValidatePasswordStrength("123456"); err != nil {
		t.Fatalf("6 位密码应通过: %v", err)
	}
	if err := ValidatePasswordStrength(""); err == nil {
		t.Fatal("空密码应被拒绝")
	}
}

func TestRandomPassword(t *testing.T) {
	p, err := RandomPassword(12)
	if err != nil {
		t.Fatalf("RandomPassword: %v", err)
	}
	if len(p) != 12 {
		t.Fatalf("期望长度 12, got %d", len(p))
	}
	// 随机口令本身必须满足强度要求
	if err := ValidatePasswordStrength(p); err != nil {
		t.Fatalf("随机口令应满足强度要求: %v", err)
	}
	p2, _ := RandomPassword(12)
	if p == p2 {
		t.Fatal("两次随机口令不应相同")
	}
}
