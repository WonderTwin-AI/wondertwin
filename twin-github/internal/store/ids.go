package store

import (
	"encoding/base64"
	"encoding/binary"
	"sync"
)

// Kind names an ID space. GitHub allocates IDs per resource type, and the
// values clients see are large: issue, comment and check-run IDs are past
// 2^31, which is exactly where a client storing them in a 32-bit int breaks.
// Each kind starts from a base of the magnitude GitHub serves today.
type Kind string

// ID spaces.
const (
	KindUser         Kind = "user"
	KindOrg          Kind = "org"
	KindRepo         Kind = "repo"
	KindIssue        Kind = "issue"
	KindPull         Kind = "pull"
	KindComment      Kind = "comment"
	KindLabel        Kind = "label"
	KindReview       Kind = "review"
	KindReviewCmt    Kind = "review_comment"
	KindStatus       Kind = "status"
	KindCheckRun     Kind = "check_run"
	KindCheckSuite   Kind = "check_suite"
	KindRelease      Kind = "release"
	KindAsset        Kind = "asset"
	KindHook         Kind = "hook"
	KindDelivery     Kind = "delivery"
	KindWorkflow     Kind = "workflow"
	KindRun          Kind = "run"
	KindJob          Kind = "job"
	KindApp          Kind = "app"
	KindInstallation Kind = "installation"
	KindMilestone    Kind = "milestone"
	KindOther        Kind = "other"
)

var idBase = map[Kind]int64{
	KindUser:         190_000_000,
	KindOrg:          210_000_000,
	KindRepo:         1_050_000_000,
	KindIssue:        3_450_000_000,
	KindPull:         2_850_000_000,
	KindComment:      3_350_000_000,
	KindLabel:        9_200_000_000,
	KindReview:       3_200_000_000,
	KindReviewCmt:    2_380_000_000,
	KindStatus:       38_000_000_000,
	KindCheckRun:     51_000_000_000,
	KindCheckSuite:   44_000_000_000,
	KindRelease:      250_000_000,
	KindAsset:        290_000_000,
	KindHook:         570_000_000,
	KindDelivery:     90_000_000_000,
	KindWorkflow:     160_000_000,
	KindRun:          18_000_000_000,
	KindJob:          51_500_000_000,
	KindApp:          1_000_000,
	KindInstallation: 88_000_000,
	KindMilestone:    13_000_000,
	KindOther:        5_000_000_000,
}

type idAllocator struct {
	mu   sync.Mutex
	next map[Kind]int64
	nums map[string]int // per-repository issue and pull request numbers
}

func newIDAllocator() *idAllocator {
	return &idAllocator{next: map[Kind]int64{}, nums: map[string]int{}}
}

func (a *idAllocator) id(k Kind) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next[k]++
	return idBase[k] + a.next[k]
}

// observe makes sure later IDs of kind k are above id, so that loaded seed
// data and freshly allocated objects never collide.
func (a *idAllocator) observe(k Kind, id int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if off := id - idBase[k]; off > a.next[k] {
		a.next[k] = off
	}
}

func (a *idAllocator) number(repoKey string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nums[repoKey]++
	return a.nums[repoKey]
}

func (a *idAllocator) observeNumber(repoKey string, n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n > a.nums[repoKey] {
		a.nums[repoKey] = n
	}
}

func (a *idAllocator) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next = map[Kind]int64{}
	a.nums = map[string]int{}
}

// NewID returns the next ID in kind's space.
func (s *MemoryStore) NewID(k Kind) int64 { return s.ids.id(k) }

// NextID returns the next ID for resources outside the kinds above.
func (s *MemoryStore) NextID() int64 { return s.ids.id(KindOther) }

// NextIssueNumber returns the next number in a repository. Issues and pull
// requests share one sequence per repository, as they do on GitHub.
func (s *MemoryStore) NextIssueNumber(owner, repo string) int {
	return s.ids.number(RepoKey(owner, repo))
}

// NodeID builds a GitHub global node ID: a type prefix, an underscore and the
// URL-safe base64 of a MessagePack array [0, ids...]. That is the shape
// GitHub has issued since 2021 (for example R_kgDO... for a repository), and
// it decodes the same way a real one does.
func NodeID(prefix string, ids ...int64) string {
	// A fixarray header holds up to 15 elements; node IDs carry two or three.
	buf := []byte{0x90 | uint8((len(ids)+1)&0x0f), 0x00}
	for _, id := range ids {
		buf = appendMsgpackUint(buf, uint64(max(id, 0)))
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf)
}

func appendMsgpackUint(b []byte, v uint64) []byte {
	switch {
	case v < 0x80:
		return append(b, uint8(v&0x7f))
	case v <= 0xff:
		return append(b, 0xcc, uint8(v&0xff))
	case v <= 0xffff:
		return binary.BigEndian.AppendUint16(append(b, 0xcd), uint16(v&0xffff))
	case v <= 0xffffffff:
		return binary.BigEndian.AppendUint32(append(b, 0xce), uint32(v&0xffffffff))
	default:
		return binary.BigEndian.AppendUint64(append(b, 0xcf), v)
	}
}
