package netmon

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// The tests run in synctest bubbles, so the waits come out exact. The wall clock is
// faked on top of the bubble's clock, because the bubble's can't jump.

var (
	corp = State{Domains: []string{"corp.example"}}
	vpn  = State{VPNHandlesAllDNS: true}
)

func TestFirstStateBeforeNew(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var returned, lateFirst atomic.Bool
		calls := 0
		onRouting := func(State) {
			calls++
			if calls == 1 && returned.Load() {
				lateFirst.Store(true)
			}
		}
		m := newMonitor((&fakeOS{state: corp}).readState, (&fakeOS{}).watch, wallClock, onRouting, func() {})
		returned.Store(true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		m.Close()
		if lateFirst.Load() {
			t.Error("first onRouting came after New returned")
		}
		if calls != 1 {
			t.Errorf("%d onRouting calls, want 1", calls)
		}
	})
}

func TestBurstGivesOneRead(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{})
		r.events()
		for range 5 {
			r.os.notify()
			time.Sleep(200 * time.Millisecond)
		}
		// The last notify was 200 ms ago.
		time.Sleep(799 * time.Millisecond)
		synctest.Wait()
		r.want(t)
		time.Sleep(time.Millisecond)
		synctest.Wait()
		r.want(t, "network")
		if n := r.os.readCount(); n != 2 {
			t.Errorf("%d reads, want 2", n)
		}
	})
}

func TestBurstWithChangedState(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{})
		r.events()
		r.os.set(corp, nil)
		r.os.notify()
		time.Sleep(settleTime)
		synctest.Wait()
		r.want(t, "routing [corp.example] false", "network")
	})
}

func TestTickRereads(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{})
		r.events()
		r.os.set(vpn, nil)
		time.Sleep(rereadInterval)
		synctest.Wait()
		r.want(t, "routing [] true")
	})
}

func TestTickAfterWake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		gap  time.Duration
		want []string
	}{
		// The routing comes first, as after a change.
		{name: "clock moved 46 s", gap: 46 * time.Second, want: []string{"routing [corp.example] false", "network"}},
		{name: "clock moved 44 s", gap: 44 * time.Second, want: []string{"routing [corp.example] false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				r := newRig(t, &fakeOS{})
				r.events()
				r.os.set(corp, nil)
				r.jump.Store(int64(tt.gap - rereadInterval))
				time.Sleep(rereadInterval)
				synctest.Wait()
				r.want(t, tt.want...)
			})
		})
	}
}

func TestReorderedDomainsAreNoChange(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{state: State{Domains: []string{"b.example", "a.example"}}})
		r.want(t, "routing [a.example b.example] false")
		r.os.set(State{Domains: []string{"a.example", "b.example", "a.example"}}, nil)
		time.Sleep(rereadInterval)
		synctest.Wait()
		r.want(t)
	})
}

func TestFailedReadKeepsState(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{state: corp})
		r.events()
		r.os.set(State{}, errors.New("configd unavailable"))
		time.Sleep(rereadInterval)
		synctest.Wait()
		r.want(t)
		// Had the failed read reported an empty state, this read would report corp again.
		r.os.set(corp, nil)
		time.Sleep(rereadInterval)
		synctest.Wait()
		r.want(t)
	})
}

// TestReadErrorLoggedOnce changes log's output, so it doesn't run in parallel.
func TestReadErrorLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	defer log.SetOutput(log.Writer())
	log.SetOutput(&buf)

	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{})
		errDown := errors.New("configd unavailable")
		// Fails twice, recovers, then fails the same way again.
		for _, err := range []error{errDown, errDown, nil, errDown} {
			r.os.set(State{}, err)
			time.Sleep(rereadInterval)
			synctest.Wait()
		}
	})
	if n := strings.Count(buf.String(), "read network state: configd unavailable"); n != 2 {
		t.Errorf("error logged %d times, want 2\n%s", n, buf.String())
	}
}

func TestTickWorksWithoutWatch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{watchErr: errors.New("no store")})
		r.events()
		r.os.set(corp, nil)
		time.Sleep(rereadInterval)
		synctest.Wait()
		r.want(t, "routing [corp.example] false")
	})
}

func TestCloseDuringSettleTime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{})
		r.events()
		r.os.set(corp, nil)
		r.os.notify()
		time.Sleep(settleTime / 2)
		r.m.Close()
		if !r.os.isStopped() {
			t.Error("watch not stopped")
		}
		time.Sleep(rereadInterval)
		synctest.Wait()
		r.want(t)
		// synctest.Test fails if the monitor's goroutine is still running.
	})
}

func TestChangeDuringFirstRead(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r := newRig(t, &fakeOS{afterFirstRead: func(o *fakeOS) {
			// The state changes after the first read took it, and the OS reports the change.
			o.set(corp, nil)
			o.notify()
		}})
		r.want(t, "routing [] false")
		time.Sleep(settleTime)
		synctest.Wait()
		r.want(t, "routing [corp.example] false", "network")
	})
}

// rig runs a monitor on a fake OS and records its callbacks.
type rig struct {
	m  *Monitor
	os *fakeOS
	// jump is how far the fake wall clock is ahead of the bubble's.
	jump atomic.Int64

	mu  sync.Mutex
	log []string
}

// newRig starts a monitor on o.
func newRig(t *testing.T, o *fakeOS) *rig {
	r := &rig{os: o}
	clock := func() time.Time {
		return time.Now().Add(time.Duration(r.jump.Load()))
	}
	r.m = newMonitor(r.os.readState, r.os.watch, clock, r.onRouting, r.onNetwork)
	// Lets the monitor's goroutine take its first clock reading before a test moves the
	// clock.
	synctest.Wait()
	t.Cleanup(func() {
		if !r.os.isStopped() {
			r.m.Close()
		}
	})
	return r
}

func (r *rig) onRouting(s State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, fmt.Sprintf("routing %v %v", s.Domains, s.VPNHandlesAllDNS))
}

func (r *rig) onNetwork() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, "network")
}

// events returns the callbacks since the last call, and forgets them.
func (r *rig) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.log
	r.log = nil
	return got
}

// want fails the test unless the callbacks since the last check are want.
func (r *rig) want(t *testing.T, want ...string) {
	t.Helper()
	if got := r.events(); !slices.Equal(got, want) {
		t.Errorf("callbacks %q, want %q", got, want)
	}
}

// fakeOS reports state, or err if set. A non-nil watchErr makes the watch fail.
// afterFirstRead, if set, runs inside the first read, after the state is taken.
type fakeOS struct {
	watchErr       error
	afterFirstRead func(*fakeOS)

	mu       sync.Mutex
	state    State
	err      error
	reads    int
	notifyFn func()
	stopped  bool
}

func (o *fakeOS) readState() (State, error) {
	o.mu.Lock()
	o.reads++
	first := o.reads == 1
	s, err := o.state, o.err
	o.mu.Unlock()
	// A copy, because the monitor sorts the domains in place.
	s.Domains = slices.Clone(s.Domains)
	if first && o.afterFirstRead != nil {
		o.afterFirstRead(o)
	}
	return s, err
}

func (o *fakeOS) watch(notify func()) (func(), error) {
	if o.watchErr != nil {
		return nil, o.watchErr
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.notifyFn = notify
	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.stopped = true
	}, nil
}

func (o *fakeOS) set(s State, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state, o.err = s, err
}

func (o *fakeOS) notify() {
	o.mu.Lock()
	notify := o.notifyFn
	o.mu.Unlock()
	notify()
}

func (o *fakeOS) readCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.reads
}

func (o *fakeOS) isStopped() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stopped
}
