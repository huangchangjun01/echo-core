package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// GenerateSalt 生成 16 字节随机盐值，并返回 hex 编码
func GenerateSalt() (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt), nil
}

// HashPassword 对明文密码加盐后再用 bcrypt 哈希
// 存储格式：bcrypt(salt + plainPassword)
func HashPassword(plain, salt string) (string, error) {
	if plain == "" {
		return "", errors.New("密码不能为空")
	}
	if salt == "" {
		return "", errors.New("盐值不能为空")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(salt+plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword 校验明文密码与已存储的哈希是否匹配
func VerifyPassword(plain, salt, hashed string) bool {
	if plain == "" || salt == "" || hashed == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(salt+plain)) == nil
}
