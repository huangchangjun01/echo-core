package remote

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"echo-core/dto"
)

// TestPythonChatClient_StreamEvents_CancelOnContextDone 验证：调用 ChatStreamEvents
// 的 ctx 被取消后,正在进行的 HTTP 请求会立即终止(远端 echo-ai 也应观察到自己
// 的 r.Context() 被取消),而不是继续跑满 http.Client.Timeout。
//
// 这个测试是任务 2.3「客户端断开 → 取消 Python 请求」的核心回归保护:
// 一旦哪天有人把 NewRequest 换回 NewRequest(无 ctx),这个测试就会失败。
func TestPythonChatClient_StreamEvents_CancelOnContextDone(t *testing.T) {
	var requestStarted atomic.Bool
	var serverObservedCancel atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStarted.Store(true)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// 模拟 echo-ai 持续产出 — 只要 ctx 没取消就一直挂着,
		// ctx 取消意味着 echo-core 已经断开 → 我们要确认 echo-ai
		// 立刻感知到客户端断开。
		select {
		case <-r.Context().Done():
			serverObservedCancel.Store(true)
		case <-time.After(3 * time.Second):
			t.Logf("mock python server safety timeout (3s)")
		}
	}))
	defer srv.Close()

	// 通过 env 把客户端的 baseURL 指到本地 mock server
	t.Setenv("ECHO_AI_REMOTE_BASE_URL", srv.URL)

	client := NewPythonChatClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handlerCalled := atomic.Int32{}
	errCh := make(chan error, 1)
	go func() {
		err := client.ChatStreamEvents(ctx, PythonChatRequest{
			UserID:    "u-cancel",
			SessionID: "s-cancel",
			Message:   "hi",
			Stream:    true,
		}, func(_ dto.ChatEvent) error {
			handlerCalled.Add(1)
			return nil
		})
		errCh <- err
	}()

	// 等待请求真的到达 Python mock
	waitFor := func(pred func() bool, label string, total time.Duration) bool {
		t.Helper()
		deadline := time.Now().Add(total)
		for time.Now().Before(deadline) {
			if pred() {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s after %s", label, total)
		return false
	}
	waitFor(requestStarted.Load, "request to reach mock server", 2*time.Second)

	// 模拟客户端断开(等价于 SSE browser close → gin.Request.Context cancel)
	cancel()

	waitFor(serverObservedCancel.Load, "mock python server to observe ctx cancel", 2*time.Second)

	// ChatStreamEvents 应该在 ctx 取消后及时返回,
	// 否则就是 http.Client.Timeout 在拖时间(默认 120s)。
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected ctx.Canceled after ctx cancel, got %v (handler called %d times)", err, handlerCalled.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ChatStreamEvents did not return within 2s after ctx cancel — HTTP request likely not bound to ctx")
	}
}