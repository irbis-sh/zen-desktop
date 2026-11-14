// Package netmon reads what the OS says about where DNS names should go, and reports
// changes to the network and wakes from sleep.
package netmon

import (
	"log"
	"slices"
	"time"
)

const (
	// settleTime is how long the OS has to stay quiet after a change before the monitor
	// reads the state. One change usually rewrites several watched keys, and the wait
	// turns them into one read and one report.
	settleTime = time.Second
	// rereadInterval is how often the state is read without a change. It catches what
	// the watch misses, such as a change of routes.
	rereadInterval = 30 * time.Second
	// wakeGap is how far the wall clock has to move between two re-reads for the monitor
	// to report a wake.
	wakeGap = 45 * time.Second
)

// State is what the network says about where names should go.
type State struct {
	// Domains holds the search and match domains: lower-cased, without trailing dots,
	// sorted and without duplicates.
	Domains []string
	// VPNHandlesAllDNS reports whether a VPN, or a DNS tool on this machine, answers every
	// name.
	VPNHandlesAllDNS bool
}

// Monitor watches the network. Create one with [New].
type Monitor struct {
	readState func() (State, error)
	// now returns the wall-clock time (see wallClock).
	now       func() time.Time
	onRouting func(State)
	onNetwork func()
	stopWatch func()
	notified  chan struct{}
	quit      chan struct{}
	done      chan struct{}

	// The fields below belong to the goroutine after New returns.

	state State
	// lastErr is the text of the last failed read's error, or empty after a good read.
	lastErr string
}

// New starts watching the network. onRouting gets the first state before New returns,
// and after that each state that differs from the last one. onNetwork is called after
// each change to the network and after each wake from sleep, after any onRouting call
// for the same change.
//
// After New returns, both callbacks run on one goroutine, one call at a time.
// [Monitor.Close] waits for that goroutine, so a callback must not call it.
func New(onRouting func(State), onNetwork func()) *Monitor {
	return newMonitor(readState, watch, wallClock, onRouting, onNetwork)
}

// wallClock returns the current time without Go's monotonic reading. A time with that
// reading subtracts by it, and on macOS and Linux it stops while the machine sleeps, so
// the gap across a sleep would look as short as the time awake.
func wallClock() time.Time {
	return time.Now().Round(0)
}

// newMonitor is [New] with the OS's functions and the clock as parameters. watch calls
// notify, from any thread, each time something changes, and the stop function it
// returns ends the watch and waits for a notify that is still running.
func newMonitor(
	readState func() (State, error),
	watch func(notify func()) (stop func(), err error),
	now func() time.Time,
	onRouting func(State),
	onNetwork func(),
) *Monitor {
	m := &Monitor{
		readState: readState,
		now:       now,
		onRouting: onRouting,
		onNetwork: onNetwork,
		notified:  make(chan struct{}, 1),
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
	}

	// The watch starts before the first read, so that a change between the two is
	// reported rather than lost until the next re-read.
	stop, err := watch(m.notify)
	if err != nil {
		log.Printf("watch network: %v, re-reading it every %s instead", err, rereadInterval)
		stop = func() {}
	}
	m.stopWatch = stop

	m.state = m.read()
	m.onRouting(m.state)

	go m.run()
	return m
}

// Close stops watching. No callback runs after it returns.
func (m *Monitor) Close() {
	close(m.quit)
	<-m.done
	m.stopWatch()
}

// notify records a change. It never blocks: one recorded change is as good as many.
func (m *Monitor) notify() {
	select {
	case m.notified <- struct{}{}:
	default:
	}
}

// run handles changes and re-reads until Close.
func (m *Monitor) run() {
	defer close(m.done)

	ticker := time.NewTicker(rereadInterval)
	defer ticker.Stop()
	lastTick := m.now()
	// settled fires once the OS has been quiet for settleTime after a change. It is nil
	// while no change is pending.
	var settled <-chan time.Time

	for {
		select {
		case <-m.notified:
			settled = time.After(settleTime)
		case <-settled:
			settled = nil
			// The routing comes first: NetworkChanged has the resolver's loop run a round
			// at once, and that round reads the VPN flag.
			m.refresh()
			// Called even when the state is the same: a new Wi-Fi network usually
			// announces the same domains as the old one, which is none.
			m.onNetwork()
		case <-ticker.C:
			// Go's timers stop during sleep on macOS and Linux, so this tick can come up to
			// rereadInterval after a wake. On Windows they keep counting.
			now := m.now()
			woke := now.Sub(lastTick) > wakeGap
			lastTick = now
			m.refresh()
			if woke {
				m.onNetwork()
			}
		case <-m.quit:
			return
		}
	}
}

// refresh reads the state and gives it to onRouting if it differs from the last one.
func (m *Monitor) refresh() {
	s := m.read()
	if s.equal(m.state) {
		return
	}
	m.state = s
	m.onRouting(s)
}

// read reads the state and sorts its domains. A failed read returns the last state: an
// empty one would send internal names to the DoH server. Before the first read, the last
// state is the empty one, because there is nothing else to give.
//
// The error is logged only when its text differs from the last one's, so that a read that
// keeps failing doesn't write a line every rereadInterval.
func (m *Monitor) read() State {
	s, err := m.readState()
	if err != nil {
		if text := err.Error(); text != m.lastErr {
			log.Printf("read network state: %v", err)
			m.lastErr = text
		}
		return m.state
	}
	m.lastErr = ""
	// Sorting here, rather than in each OS's reader, keeps them from disagreeing on what
	// counts as a change.
	slices.Sort(s.Domains)
	s.Domains = slices.Compact(s.Domains)
	return s
}

func (s State) equal(other State) bool {
	return s.VPNHandlesAllDNS == other.VPNHandlesAllDNS && slices.Equal(s.Domains, other.Domains)
}
