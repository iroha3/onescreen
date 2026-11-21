package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// logFile 是常驻日志：托盘程序没有控制台，出问题只能靠它。
type logFile struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

func newLog(dir string) *logFile {
	l := &logFile{path: filepath.Join(dir, "switch.log")}
	l.open()
	return l
}

func (l *logFile) open() {
	if st, err := os.Stat(l.path); err == nil && st.Size() > 2<<20 {
		_ = os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	l.f = f
}

func (l *logFile) Printf(format string, args ...any) {
	line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		_, _ = l.f.WriteString(line)
	}
	fmt.Print(line) // 从终端启动时也能直接看到
}

func (l *logFile) Path() string { return l.path }
