//go:build linux

package term

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// openPtyPair returns the master side of a new pseudo-terminal and the path
// of its slave side, the "terminal" kmd queries.
func openPtyPair(t *testing.T) (*os.File, string) {
	t.Helper()
	master := openPty(t)
	var unlock int32
	var n uint32
	for req, arg := range map[uintptr]unsafe.Pointer{syscall.TIOCSPTLCK: unsafe.Pointer(&unlock), syscall.TIOCGPTN: unsafe.Pointer(&n)} {
		// #nosec G103 -- the pty ioctls require pointers.
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), req, uintptr(arg)); e != 0 {
			t.Skipf("pty ioctl: %v", e)
		}
	}
	path := "/dev/pts/" + strconv.Itoa(int(n))
	// held open so reading the master does not fail with EIO before the
	// code under test opens the slave
	slave, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, path
}

// fakeTerminal answers the background query on master: it writes each
// part of the answer, pausing for pause before every part after the first,
// and then sends what it read.
func fakeTerminal(t *testing.T, master *os.File, pause time.Duration, parts ...string) <-chan []byte {
	t.Helper()
	got := make(chan []byte, 1)
	go func() {
		var in []byte
		b := make([]byte, 64)
		for !bytes.Contains(in, []byte(backgroundQuery)) {
			n, err := master.Read(b)
			if err != nil {
				break
			}
			in = append(in, b[:n]...)
		}
		for i, p := range parts {
			if i > 0 {
				time.Sleep(pause)
			}
			_, _ = master.WriteString(p)
		}
		got <- in
	}()
	return got
}

// receive waits for what the fake terminal read, failing instead of
// hanging when the query never came.
func receive(t *testing.T, sent <-chan []byte) []byte {
	t.Helper()
	select {
	case q := <-sent:
		return q
	case <-time.After(testlimit.Scale() * 5 * time.Second):
		t.Fatal("the query never reached the terminal")
		return nil
	}
}

// termiosOf reads the mode of the terminal at path.
func termiosOf(t *testing.T, path string) syscall.Termios {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var tio syscall.Termios
	if err := termios(f.Fd(), ioctlGet, &tio); err != nil {
		t.Fatal(err)
	}
	return tio
}

func TestLightBackgroundPty(t *testing.T) {
	const da1 = "\x1b[?62;22c"
	for _, tc := range []struct {
		name      string
		reply     []string // written with a pause between parts
		light, ok bool
	}{
		{"light", []string{"\x1b]11;rgb:fdfd/f6f6/e3e3\x1b\\" + da1}, true, true},
		{"dark", []string{"\x1b]11;rgb:1a1a/1b1b/2626\a" + da1}, false, true},
		{"no OSC 11", []string{da1}, false, false},
		{"hostile", []string{"\x1b]11;rgb:ffff/ffff/ffff\x1b]0;pwned\a" + da1}, false, false},
		// an answer that has begun is read to the end, so none of it is
		// left for the shell or the viewer
		{"slow", []string{"\x1b]11;rgb:ffff/", "ffff/ffff\a\x1b[?6", "2c"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			master, slave := openPtyPair(t)
			before := termiosOf(t, slave)
			sent := fakeTerminal(t, master, 300*time.Millisecond, tc.reply...)
			light, ok := LightBackground(slave, 100*time.Millisecond)
			if light != tc.light || ok != tc.ok {
				t.Fatalf("LightBackground = %v,%v; want %v,%v", light, ok, tc.light, tc.ok)
			}
			if q := receive(t, sent); string(q) != backgroundQuery {
				t.Errorf("sent %q", q)
			}
			if after := termiosOf(t, slave); after != before {
				t.Errorf("terminal mode not restored:\n%+v\n%+v", before, after)
			}
		})
	}
}

func TestQueryBackgroundSilentTerminal(t *testing.T) {
	master, slave := openPtyPair(t)
	before := termiosOf(t, slave)
	fakeTerminal(t, master, 0)
	start := time.Now()
	reply, err := queryBackground(slave, 100*time.Millisecond)
	if !errors.Is(err, errNoReply) || len(reply) != 0 {
		t.Fatalf("queryBackground = %q, %v", reply, err)
	}
	if d := time.Since(start); d > testlimit.Scale()*time.Second {
		t.Errorf("waited %v", d)
	}
	if after := termiosOf(t, slave); after != before {
		t.Error("terminal mode not restored")
	}
}

func TestQueryBackgroundBounded(t *testing.T) {
	// the writers stop when the test's cleanup closes the master
	t.Run("size", func(t *testing.T) {
		master, slave := openPtyPair(t)
		go func() { // a terminal that never stops talking
			b := make([]byte, 64)
			_, _ = master.Read(b)
			junk := bytes.Repeat([]byte("x"), 64)
			for {
				if _, err := master.Write(junk); err != nil {
					return
				}
			}
		}()
		reply, err := queryBackground(slave, time.Second)
		if err != nil || len(reply) != maxReply {
			t.Fatalf("queryBackground = %d bytes, %v", len(reply), err)
		}
	})
	t.Run("time", func(t *testing.T) {
		master, slave := openPtyPair(t)
		go func() { // a trickle that never completes the answer
			b := make([]byte, 64)
			_, _ = master.Read(b)
			for {
				if _, err := master.WriteString("x"); err != nil {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
		start := time.Now()
		if _, err := queryBackground(slave, 100*time.Millisecond); !errors.Is(err, errNoReply) {
			t.Fatalf("err = %v", err)
		}
		if d := time.Since(start); d < MaxQueryWait || d > MaxQueryWait+testlimit.Scale()*time.Second {
			t.Errorf("waited %v; want about %v", d, MaxQueryWait)
		}
	})
}

func TestQueryBackgroundNotATerminal(t *testing.T) {
	if _, ok := LightBackground(os.DevNull, time.Second); ok {
		t.Fatal("answer from /dev/null")
	}
	if _, ok := LightBackground("/nonexistent/tty", time.Second); ok {
		t.Fatal("answer from a missing file")
	}
}

func TestTenths(t *testing.T) {
	for d, want := range map[time.Duration]uint8{0: 1, 40 * time.Millisecond: 1, 100 * time.Millisecond: 1,
		160 * time.Millisecond: 2, time.Second: 10, time.Hour: 255} {
		if got := tenths(d); got != want {
			t.Errorf("tenths(%v) = %d; want %d", d, got, want)
		}
	}
}

// TestQueryRestoresOnSignal kills a process waiting for the terminal's
// answer and checks that the terminal is no longer in raw mode.
func TestQueryRestoresOnSignal(t *testing.T) {
	if path := os.Getenv("KMD_TERM_QUERY_CHILD"); path != "" {
		_, _ = queryBackground(path, time.Minute)
		os.Exit(0) // not reached: killed by the parent
	}
	master, slave := openPtyPair(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestQueryRestoresOnSignal$") // #nosec G204,G702 -- the test binary itself
	cmd.Env = append(os.Environ(), "KMD_TERM_QUERY_CHILD="+slave)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() }) // if the test fails before the signal
	receive(t, fakeTerminal(t, master, 0))       // the query is out, so the terminal is raw
	if raw := termiosOf(t, slave); raw.Lflag&syscall.ICANON != 0 {
		t.Fatal("terminal not in raw mode during the query")
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	err := cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
		t.Fatalf("child: %v; want killed by SIGTERM", err)
	}
	if after := termiosOf(t, slave); after.Lflag&(syscall.ICANON|syscall.ECHO) != syscall.ICANON|syscall.ECHO {
		t.Error("terminal left in raw mode")
	}
}
