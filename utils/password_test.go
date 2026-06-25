package utils

import (
	"testing"
)

// TestGenerateSalt 测试盐值生成
func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error = %v", err)
	}
	if len(salt) != 32 {
		t.Errorf("GenerateSalt() length = %d, want 32 (hex编码16字节)", len(salt))
	}

	// 两次生成的盐值应不同
	salt2, _ := GenerateSalt()
	if salt == salt2 {
		t.Error("GenerateSalt() 两次生成应不同")
	}
}

// TestHashPassword 测试密码哈希
func TestHashPassword(t *testing.T) {
	tests := []struct {
		name    string
		plain   string
		salt    string
		wantErr bool
	}{
		{"正常密码", "MyPassword123", "abcdef1234567890abcdef1234567890", false},
		{"空密码", "", "abcdef1234567890abcdef1234567890", true},
		{"空盐值", "MyPassword123", "", true},
		{"简短密码", "ab", "abcdef1234567890abcdef1234567890", false},
		{"特殊字符密码", "!@#$%^&*()_+-=[]{}|;':\",./<>?", "abcdef1234567890abcdef1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.plain, tt.salt)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("HashPassword() 返回空哈希")
			}
		})
	}
}

// TestVerifyPassword 测试密码校验
func TestVerifyPassword(t *testing.T) {
	plain := "CorrectPassword123"
	salt, _ := GenerateSalt()
	hash, _ := HashPassword(plain, salt)

	tests := []struct {
		name    string
		plain   string
		salt    string
		hashed  string
		want    bool
	}{
		{"正确密码", plain, salt, hash, true},
		{"错误密码", "WrongPassword", salt, hash, false},
		{"空明文", "", salt, hash, false},
		{"空盐值", plain, "", hash, false},
		{"空哈希", plain, salt, "", false},
		{"不同盐值", plain, "different_salt_value_1234567890", hash, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyPassword(tt.plain, tt.salt, tt.hashed); got != tt.want {
				t.Errorf("VerifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHashAndVerifyRoundtrip 测试哈希与校验的往返
func TestHashAndVerifyRoundtrip(t *testing.T) {
	passwords := []string{"Hello123!", "Test@2024", "密码测试123", "a", "!@#$%^&*()"}

	for _, pw := range passwords {
		t.Run("", func(t *testing.T) {
			salt, err := GenerateSalt()
			if err != nil {
				t.Fatal(err)
			}
			hash, err := HashPassword(pw, salt)
			if err != nil {
				t.Fatal(err)
			}
			if !VerifyPassword(pw, salt, hash) {
				t.Errorf("往返校验失败: password=%s", pw)
			}
		})
	}
}