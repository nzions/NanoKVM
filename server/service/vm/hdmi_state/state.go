package hdmi_state

type viewerSource struct {
	count   int
	version uint64
}

// State stores capture demand. Callers must serialize access to State.
type State struct {
	viewerCount     int
	leaseCount      int
	sources         map[string]viewerSource
	warmAfterSeq    uint64
	needsFreshFrame bool
}

func New() State {
	return State{sources: make(map[string]viewerSource)}
}

// UpdateViewer accepts only newer source snapshots so a delayed report cannot
// overwrite a newer client-map snapshot.
func (s *State) UpdateViewer(source string, count int, version uint64) bool {
	if count < 0 {
		count = 0
	}
	if current, ok := s.sources[source]; ok && version <= current.version {
		return false
	}

	s.sources[source] = viewerSource{count: count, version: version}
	s.viewerCount = 0
	for _, current := range s.sources {
		s.viewerCount += current.count
	}
	return true
}

func (s *State) NextVersion(source string) uint64 {
	return s.sources[source].version + 1
}

func (s *State) AcquireLease() {
	s.leaseCount++
}

func (s *State) ReleaseLease() bool {
	if s.leaseCount == 0 {
		return false
	}
	s.leaseCount--
	return true
}

func (s *State) HasDemand() bool {
	return s.viewerCount > 0 || s.leaseCount > 0
}

func (s *State) ViewerCount() int {
	return s.viewerCount
}

func (s *State) LeaseCount() int {
	return s.leaseCount
}

// MarkWarming records that capture was just (re)enabled. afterSeq is the HDMI
// snapshot seq current at that moment: capture is warm once a snapshot above
// it reports Streaming or NoSignal.
func (s *State) MarkWarming(afterSeq uint64) {
	s.warmAfterSeq = afterSeq
	s.needsFreshFrame = true
}

func (s *State) ClearWarming() {
	s.warmAfterSeq = 0
	s.needsFreshFrame = false
}

func (s *State) WarmAfterSeq() uint64 {
	return s.warmAfterSeq
}

func (s *State) NeedsFreshFrame() bool {
	return s.needsFreshFrame
}

// ClaimFreshFrame returns true for the first successful reader after HDMI
// resumes. Failed reads do not consume the claim.
func (s *State) ClaimFreshFrame() bool {
	if !s.needsFreshFrame {
		return false
	}
	s.needsFreshFrame = false
	return true
}

func (s *State) Viewer(source string) (count int, version uint64, ok bool) {
	value, ok := s.sources[source]
	return value.count, value.version, ok
}
