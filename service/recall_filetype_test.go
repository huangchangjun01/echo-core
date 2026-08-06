package service

import "testing"

// 回归测试：源文件 fileType 的服务端权威归一。
//
// 背景（用户反馈的视频解析 bug）：
//
//	记忆编辑态下，前端把已有源文件用空 File 占位，保存时按 File.type 重新推导 fileType，
//	导致 video(3) 退化成 text(1)。echo-core 原样透传给 echo-ai 后，registry 分派到
//	parse_text，把 mp4 原始字节 decode 成文本喂给 LLM，md 里出现 "MP4 容器元数据"。
//
// 服务端不能只当搬运工：文件名扩展名与 DB 历史值都是比"请求值"更可信的证据。
// 权威顺序：扩展名 > DB 旧值 > 请求值。
func TestResolveSourceFileType(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		reqType  int
		dbType   int
		want     int
	}{
		// —— 核心回归：编辑态把 mp4 降级成文本 ——
		{"mp4 被前端降级为文本，按扩展名纠正", "拉师傅的欢乐时光.mp4", 1, 3, 3},
		{"mp4 无 DB 旧值（首次保存）也按扩展名纠正", "clip.mp4", 1, 0, 3},

		// —— 其它模态同样纠正 ——
		{"mov 视频", "clip.mov", 1, 0, 3},
		{"mkv 视频", "clip.mkv", 2, 0, 3},
		{"mp3 音频", "song.mp3", 1, 0, 4},
		{"wav 音频", "voice.wav", 3, 0, 4},
		{"jpg 图片", "photo.jpg", 1, 0, 2},
		{"png 图片", "photo.PNG", 4, 0, 2},
		{"txt 文本", "note.txt", 3, 0, 1},
		{"md 文本", "readme.md", 2, 0, 1},

		// —— 扩展名不可识别：退回 DB 旧值 ——
		{"未知扩展名优先用 DB 旧值", "backup.bin", 1, 3, 3},
		{"未知扩展名且无 DB 旧值，用请求值", "backup.bin", 3, 0, 3},
		{"无扩展名优先用 DB 旧值", "noext", 1, 4, 4},

		// —— 正常链路不被破坏 ——
		{"请求值正确时保持不变", "clip.mp4", 3, 3, 3},
		{"文本文件请求值正确", "note.txt", 1, 1, 1},

		// —— 兜底 ——
		{"全部不可判定时返回文本", "backup.bin", 0, 0, 1},
		{"请求值越界时按扩展名纠正", "clip.mp4", 99, 0, 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveSourceFileType(c.fileName, c.reqType, c.dbType)
			if got != c.want {
				t.Fatalf("resolveSourceFileType(%q, req=%d, db=%d) = %d, want %d",
					c.fileName, c.reqType, c.dbType, got, c.want)
			}
		})
	}
}

// TestInferFileTypeByExt 扩展名推断本身的边界。
func TestInferFileTypeByExt(t *testing.T) {
	cases := map[string]int{
		"a.mp4":         3,
		"A.MP4":         3,
		"a.jpeg":        2,
		"a.flac":        4,
		"a.json":        1,
		"a.bin":         0,
		"noext":         0,
		"":              0,
		"归档.tar.gz":     0,
		"视频.文件.mp4":     3,
		"trailing.mp4 ": 0, // 尾随空格会让 path.Ext 拿到 ".mp4 "，非命中（小概率边角，不强求）
	}
	for name, want := range cases {
		if got := inferFileTypeByExt(name); got != want {
			t.Errorf("inferFileTypeByExt(%q) = %d, want %d", name, got, want)
		}
	}
}
