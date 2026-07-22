package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// FileConfig JSON字段
type FileConfig map[string]interface{}

// MarshalConfig 将 FileConfig 序列化为 JSON 字符串
func MarshalConfig(c FileConfig) string {
	if c == nil {
		return "{}"
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// UnmarshalConfig 从 JSON 字符串反序列化为 FileConfig
func UnmarshalConfig(data string) FileConfig {
	if data == "" {
		return FileConfig{}
	}
	var c FileConfig
	err := json.Unmarshal([]byte(data), &c)
	if err != nil {
		return FileConfig{}
	}
	return c
}

// Value 实现 driver.Valuer 接口，用于写入数据库
func (c *FileConfig) Value() (interface{}, error) {
	if c == nil || *c == nil {
		return "{}", nil
	}
	data, err := json.Marshal(*c)
	if err != nil {
		return "{}", err
	}
	return string(data), nil
}

// Scan 实现 sql.Scanner 接口，用于从数据库读取
func (c *FileConfig) Scan(value interface{}) error {
	if value == nil {
		*c = FileConfig{}
		return nil
	}
	var data string
	switch v := value.(type) {
	case []byte:
		data = string(v)
	case string:
		data = v
	default:
		return fmt.Errorf("cannot scan type %T into FileConfig", value)
	}
	*c = UnmarshalConfig(data)
	return nil
}

// File 文件实体
type File struct {
	Id        uint        `json:"id" gorm:"primaryKey"`
	Name      string      `json:"name" gorm:"column:name;size:255;comment:文件名称"`
	UserId    string      `json:"user_id" gorm:"column:user_id;size:255;comment:用户id"`
	Key       string      `json:"key" gorm:"column:key;size:255;comment:文件Key"`
	FileType  int         `json:"file_type" gorm:"column:file_type;comment:1-文本，2-图片，3-视频，4-音频"`
	BizType   int         `json:"biz_type" gorm:"column:biz_type"`
	Status    int         `json:"status" gorm:"column:status;comment:1-可用，2-已删除"`
	Desc      string      `json:"desc" gorm:"column:desc;type:text;comment:文件/文本描述"`
	RoleId    string      `json:"roleId" gorm:"column:role_id;size:128;index;comment:角色ID"`
	Config    *FileConfig `json:"-" gorm:"column:config;type:text;serializer:json;comment:文件配置JSON"`
	CreatedAt time.Time   `json:"created_at" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time   `json:"updated_at" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
}

func (File) TableName() string {
	return "file"
}
