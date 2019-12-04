// Copyright 2016-2019 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kubemq-io/broker/nuid"
)

// FIXME(dlc) - this is also used by monitor_test. Not needed with t.Helper.
func stackFatalf(t *testing.T, f string, args ...interface{}) {
	lines := make([]string, 0, 32)
	msg := fmt.Sprintf(f, args...)
	lines = append(lines, msg)

	// Generate the Stack of callers: Skip us and verify* frames.
	for i := 2; true; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		msg := fmt.Sprintf("%d - %s:%d", i, file, line)
		lines = append(lines, msg)
	}
	t.Fatalf("%s", strings.Join(lines, "\n"))
}

func verifyCount(s *Sublist, count uint32, t *testing.T) {
	t.Helper()
	if s.Count() != count {
		t.Fatalf("Count is %d, should be %d", s.Count(), count)
	}
}

func verifyLen(r []*subscription, l int, t *testing.T) {
	t.Helper()
	if len(r) != l {
		t.Fatalf("Results len is %d, should be %d", len(r), l)
	}
}

func verifyQLen(r [][]*subscription, l int, t *testing.T) {
	t.Helper()
	if len(r) != l {
		t.Fatalf("Queue Results len is %d, should be %d", len(r), l)
	}
}

func verifyNumLevels(s *Sublist, expected int, t *testing.T) {
	t.Helper()
	dl := s.numLevels()
	if dl != expected {
		t.Fatalf("NumLevels is %d, should be %d", dl, expected)
	}
}

func verifyQMember(qsubs [][]*subscription, val *subscription, t *testing.T) {
	t.Helper()
	verifyMember(qsubs[findQSlot(val.queue, qsubs)], val, t)
}

func verifyMember(r []*subscription, val *subscription, t *testing.T) {
	t.Helper()
	for _, v := range r {
		if v == nil {
			continue
		}
		if v == val {
			return
		}
	}
	t.Fatalf("Subscription (%p) for [%s : %s] not found in results", val, val.subject, val.queue)
}

// Helpers to generate test subscriptions.
func newSub(subject string) *subscription {
	c := &client{kind: CLIENT}
	return &subscription{client: c, subject: []byte(subject)}
}

func newQSub(subject, queue string) *subscription {
	if queue != "" {
		return &subscription{subject: []byte(subject), queue: []byte(queue)}
	}
	return newSub(subject)
}

func newRemoteQSub(subject, queue string, num int32) *subscription {
	if queue != "" {
		c := &client{kind: ROUTER}
		return &subscription{client: c, subject: []byte(subject), queue: []byte(queue), qw: num}
	}
	return newSub(subject)
}

func TestSublistInit(t *testing.T) {
	s := NewSublistWithCache()
	verifyCount(s, 0, t)
}

func TestSublistInsertCount(t *testing.T) {
	testSublistInsertCount(t, NewSublistWithCache())
}

func TestSublistInsertCountNoCache(t *testing.T) {
	testSublistInsertCount(t, NewSublistNoCache())
}

func testSublistInsertCount(t *testing.T, s *Sublist) {
	s.Insert(newSub("foo"))
	s.Insert(newSub("bar"))
	s.Insert(newSub("foo.bar"))
	verifyCount(s, 3, t)
}

func TestSublistSimple(t *testing.T) {
	testSublistSimple(t, NewSublistWithCache())
}

func TestSublistSimpleNoCache(t *testing.T) {
	testSublistSimple(t, NewSublistNoCache())
}

func testSublistSimple(t *testing.T, s *Sublist) {
	subject := "foo"
	sub := newSub(subject)
	s.Insert(sub)
	r := s.Match(subject)
	verifyLen(r.psubs, 1, t)
	verifyMember(r.psubs, sub, t)
}

func TestSublistSimpleMultiTokens(t *testing.T) {
	testSublistSimpleMultiTokens(t, NewSublistWithCache())
}

func TestSublistSimpleMultiTokensNoCache(t *testing.T) {
	testSublistSimpleMultiTokens(t, NewSublistNoCache())
}

func testSublistSimpleMultiTokens(t *testing.T, s *Sublist) {
	subject := "foo.bar.baz"
	sub := newSub(subject)
	s.Insert(sub)
	r := s.Match(subject)
	verifyLen(r.psubs, 1, t)
	verifyMember(r.psubs, sub, t)
}

func TestSublistPartialWildcard(t *testing.T) {
	testSublistPartialWildcard(t, NewSublistWithCache())
}

func TestSublistPartialWildcardNoCache(t *testing.T) {
	testSublistPartialWildcard(t, NewSublistNoCache())
}

func testSublistPartialWildcard(t *testing.T, s *Sublist) {
	lsub := newSub("a.b.c")
	psub := newSub("a.*.c")
	s.Insert(lsub)
	s.Insert(psub)
	r := s.Match("a.b.c")
	verifyLen(r.psubs, 2, t)
	verifyMember(r.psubs, lsub, t)
	verifyMember(r.psubs, psub, t)
}

func TestSublistPartialWildcardAtEnd(t *testing.T) {
	testSublistPartialWildcardAtEnd(t, NewSublistWithCache())
}

func TestSublistPartialWildcardAtEndNoCache(t *testing.T) {
	testSublistPartialWildcardAtEnd(t, NewSublistNoCache())
}

func testSublistPartialWildcardAtEnd(t *testing.T, s *Sublist) {
	lsub := newSub("a.b.c")
	psub := newSub("a.b.*")
	s.Insert(lsub)
	s.Insert(psub)
	r := s.Match("a.b.c")
	verifyLen(r.psubs, 2, t)
	verifyMember(r.psubs, lsub, t)
	verifyMember(r.psubs, psub, t)
}

func TestSublistFullWildcard(t *testing.T) {
	testSublistFullWildcard(t, NewSublistWithCache())
}

func TestSublistFullWildcardNoCache(t *testing.T) {
	testSublistFullWildcard(t, NewSublistNoCache())
}

func testSublistFullWildcard(t *testing.T, s *Sublist) {
	lsub := newSub("a.b.c")
	fsub := newSub("a.>")
	s.Insert(lsub)
	s.Insert(fsub)
	r := s.Match("a.b.c")
	verifyLen(r.psubs, 2, t)
	verifyMember(r.psubs, lsub, t)
	verifyMember(r.psubs, fsub, t)
}

func TestSublistRemove(t *testing.T) {
	testSublistRemove(t, NewSublistWithCache())
}

func TestSublistRemoveNoCache(t *testing.T) {
	testSublistRemove(t, NewSublistNoCache())
}

func testSublistRemove(t *testing.T, s *Sublist) {
	subject := "a.b.c.d"
	sub := newSub(subject)
	s.Insert(sub)
	verifyCount(s, 1, t)
	r := s.Match(subject)
	verifyLen(r.psubs, 1, t)
	s.Remove(newSub("a.b.c"))
	verifyCount(s, 1, t)
	s.Remove(sub)
	verifyCount(s, 0, t)
	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
}

func TestSublistRemoveWildcard(t *testing.T) {
	testSublistRemoveWildcard(t, NewSublistWithCache())
}

func TestSublistRemoveWildcardNoCache(t *testing.T) {
	testSublistRemoveWildcard(t, NewSublistNoCache())
}

func testSublistRemoveWildcard(t *testing.T, s *Sublist) {
	subject := "a.b.c.d"
	sub := newSub(subject)
	psub := newSub("a.b.*.d")
	fsub := newSub("a.b.>")
	s.Insert(sub)
	s.Insert(psub)
	s.Insert(fsub)
	verifyCount(s, 3, t)
	r := s.Match(subject)
	verifyLen(r.psubs, 3, t)
	s.Remove(sub)
	verifyCount(s, 2, t)
	s.Remove(fsub)
	verifyCount(s, 1, t)
	s.Remove(psub)
	verifyCount(s, 0, t)
	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
}

func TestSublistRemoveCleanup(t *testing.T) {
	testSublistRemoveCleanup(t, NewSublistWithCache())
}

func TestSublistRemoveCleanupNoCache(t *testing.T) {
	testSublistRemoveCleanup(t, NewSublistNoCache())
}

func testSublistRemoveCleanup(t *testing.T, s *Sublist) {
	literal := "a.b.c.d.e.f"
	depth := len(strings.Split(literal, tsep))
	sub := newSub(literal)
	verifyNumLevels(s, 0, t)
	s.Insert(sub)
	verifyNumLevels(s, depth, t)
	s.Remove(sub)
	verifyNumLevels(s, 0, t)
}

func TestSublistRemoveCleanupWildcards(t *testing.T) {
	testSublistRemoveCleanupWildcards(t, NewSublistWithCache())
}

func TestSublistRemoveCleanupWildcardsNoCache(t *testing.T) {
	testSublistRemoveCleanupWildcards(t, NewSublistNoCache())
}

func testSublistRemoveCleanupWildcards(t *testing.T, s *Sublist) {
	subject := "a.b.*.d.e.>"
	depth := len(strings.Split(subject, tsep))
	sub := newSub(subject)
	verifyNumLevels(s, 0, t)
	s.Insert(sub)
	verifyNumLevels(s, depth, t)
	s.Remove(sub)
	verifyNumLevels(s, 0, t)
}

func TestSublistRemoveWithLargeSubs(t *testing.T) {
	testSublistRemoveWithLargeSubs(t, NewSublistWithCache())
}

func TestSublistRemoveWithLargeSubsNoCache(t *testing.T) {
	testSublistRemoveWithLargeSubs(t, NewSublistNoCache())
}

func testSublistRemoveWithLargeSubs(t *testing.T, s *Sublist) {
	subject := "foo"
	for i := 0; i < plistMin*2; i++ {
		sub := newSub(subject)
		s.Insert(sub)
	}
	r := s.Match(subject)
	verifyLen(r.psubs, plistMin*2, t)
	// Remove one that is in the middle
	s.Remove(r.psubs[plistMin])
	// Remove first one
	s.Remove(r.psubs[0])
	// Remove last one
	s.Remove(r.psubs[len(r.psubs)-1])
	// Check len again
	r = s.Match(subject)
	verifyLen(r.psubs, plistMin*2-3, t)
}

func TestSublistRemoveByClient(t *testing.T) {
	testSublistRemoveByClient(t, NewSublistWithCache())
}

func TestSublistRemoveByClientNoCache(t *testing.T) {
	testSublistRemoveByClient(t, NewSublistNoCache())
}

func testSublistRemoveByClient(t *testing.T, s *Sublist) {
	c := &client{}
	for i := 0; i < 10; i++ {
		subject := fmt.Sprintf("a.b.c.d.e.f.%d", i)
		sub := &subscription{client: c, subject: []byte(subject)}
		s.Insert(sub)
	}
	verifyCount(s, 10, t)
	s.Insert(&subscription{client: c, subject: []byte(">")})
	s.Insert(&subscription{client: c, subject: []byte("foo.*")})
	s.Insert(&subscription{client: c, subject: []byte("foo"), queue: []byte("bar")})
	s.Insert(&subscription{client: c, subject: []byte("foo"), queue: []byte("bar")})
	s.Insert(&subscription{client: c, subject: []byte("foo.bar"), queue: []byte("baz")})
	s.Insert(&subscription{client: c, subject: []byte("foo.bar"), queue: []byte("baz")})
	verifyCount(s, 16, t)
	genid := atomic.LoadUint64(&s.genid)
	s.RemoveAllForClient(c)
	verifyCount(s, 0, t)
	// genid should be different
	if genid == atomic.LoadUint64(&s.genid) {
		t.Fatalf("GenId should have been changed after removal of subs")
	}
	if s.CacheEnabled() {
		if cc := s.CacheCount(); cc != 0 {
			t.Fatalf("Cache should be zero, got %d", cc)
		}
	}
}

func TestSublistInvalidSubjectsInsert(t *testing.T) {
	testSublistInvalidSubjectsInsert(t, NewSublistWithCache())
}

func TestSublistInvalidSubjectsInsertNoCache(t *testing.T) {
	testSublistInvalidSubjectsInsert(t, NewSublistNoCache())
}

func testSublistInvalidSubjectsInsert(t *testing.T, s *Sublist) {
	// Insert, or subscriptions, can have wildcards, but not empty tokens,
	// and can not have a FWC that is not the terminal token.

	// beginning empty token
	if err := s.Insert(newSub(".foo")); err != ErrInvalidSubject {
		t.Fatal("Expected invalid subject error")
	}

	// trailing empty token
	if err := s.Insert(newSub("foo.")); err != ErrInvalidSubject {
		t.Fatal("Expected invalid subject error")
	}
	// empty middle token
	if err := s.Insert(newSub("foo..bar")); err != ErrInvalidSubject {
		t.Fatal("Expected invalid subject error")
	}
	// empty middle token #2
	if err := s.Insert(newSub("foo.bar..baz")); err != ErrInvalidSubject {
		t.Fatal("Expected invalid subject error")
	}
	// fwc not terminal
	if err := s.Insert(newSub("foo.>.bar")); err != ErrInvalidSubject {
		t.Fatal("Expected invalid subject error")
	}
}

func TestSublistCache(t *testing.T) {
	s := NewSublistWithCache()

	// Test add a remove logistics
	subject := "a.b.c.d"
	sub := newSub(subject)
	psub := newSub("a.b.*.d")
	fsub := newSub("a.b.>")
	s.Insert(sub)
	r := s.Match(subject)
	verifyLen(r.psubs, 1, t)
	s.Insert(psub)
	s.Insert(fsub)
	verifyCount(s, 3, t)
	r = s.Match(subject)
	verifyLen(r.psubs, 3, t)
	s.Remove(sub)
	verifyCount(s, 2, t)
	s.Remove(fsub)
	verifyCount(s, 1, t)
	s.Remove(psub)
	verifyCount(s, 0, t)

	// Check that cache is now empty
	if cc := s.CacheCount(); cc != 0 {
		t.Fatalf("Cache should be zero, got %d\n", cc)
	}

	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)

	for i := 0; i < 2*slCacheMax; i++ {
		s.Match(fmt.Sprintf("foo-%d\n", i))
	}

	checkFor(t, 2*time.Second, 10*time.Millisecond, func() error {
		if cc := s.CacheCount(); cc > slCacheMax {
			return fmt.Errorf("Cache should be constrained by cacheMax, got %d for current count", cc)
		}
		return nil
	})

	// Test that adding to a wildcard properly adds to the cache.
	s = NewSublistWithCache()
	s.Insert(newSub("foo.*"))
	s.Insert(newSub("foo.bar"))
	r = s.Match("foo.baz")
	verifyLen(r.psubs, 1, t)
	r = s.Match("foo.bar")
	verifyLen(r.psubs, 2, t)
	s.Insert(newSub("foo.>"))
	r = s.Match("foo.bar")
	verifyLen(r.psubs, 3, t)
}

func TestSublistBasicQueueResults(t *testing.T) {
	testSublistBasicQueueResults(t, NewSublistWithCache())
}

func TestSublistBasicQueueResultsNoCache(t *testing.T) {
	testSublistBasicQueueResults(t, NewSublistNoCache())
}

func testSublistBasicQueueResults(t *testing.T, s *Sublist) {
	// Test some basics
	subject := "foo"
	sub := newSub(subject)
	sub1 := newQSub(subject, "bar")
	sub2 := newQSub(subject, "baz")

	s.Insert(sub1)
	r := s.Match(subject)
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 1, t)
	verifyLen(r.qsubs[0], 1, t)
	verifyQMember(r.qsubs, sub1, t)

	s.Insert(sub2)
	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 2, t)
	verifyLen(r.qsubs[0], 1, t)
	verifyLen(r.qsubs[1], 1, t)
	verifyQMember(r.qsubs, sub1, t)
	verifyQMember(r.qsubs, sub2, t)

	s.Insert(sub)
	r = s.Match(subject)
	verifyLen(r.psubs, 1, t)
	verifyQLen(r.qsubs, 2, t)
	verifyLen(r.qsubs[0], 1, t)
	verifyLen(r.qsubs[1], 1, t)
	verifyQMember(r.qsubs, sub1, t)
	verifyQMember(r.qsubs, sub2, t)
	verifyMember(r.psubs, sub, t)

	sub3 := newQSub(subject, "bar")
	sub4 := newQSub(subject, "baz")

	s.Insert(sub3)
	s.Insert(sub4)

	r = s.Match(subject)
	verifyLen(r.psubs, 1, t)
	verifyQLen(r.qsubs, 2, t)
	verifyLen(r.qsubs[0], 2, t)
	verifyLen(r.qsubs[1], 2, t)
	verifyQMember(r.qsubs, sub1, t)
	verifyQMember(r.qsubs, sub2, t)
	verifyQMember(r.qsubs, sub3, t)
	verifyQMember(r.qsubs, sub4, t)
	verifyMember(r.psubs, sub, t)

	// Now removal
	s.Remove(sub)

	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 2, t)
	verifyLen(r.qsubs[0], 2, t)
	verifyLen(r.qsubs[1], 2, t)
	verifyQMember(r.qsubs, sub1, t)
	verifyQMember(r.qsubs, sub2, t)

	s.Remove(sub1)
	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 2, t)
	verifyLen(r.qsubs[findQSlot(sub1.queue, r.qsubs)], 1, t)
	verifyLen(r.qsubs[findQSlot(sub2.queue, r.qsubs)], 2, t)
	verifyQMember(r.qsubs, sub2, t)
	verifyQMember(r.qsubs, sub3, t)
	verifyQMember(r.qsubs, sub4, t)

	s.Remove(sub3) // Last one
	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 1, t)
	verifyLen(r.qsubs[0], 2, t) // this is sub2/baz now
	verifyQMember(r.qsubs, sub2, t)

	s.Remove(sub2)
	s.Remove(sub4)
	r = s.Match(subject)
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 0, t)
}

func checkBool(b, expected bool, t *testing.T) {
	t.Helper()
	if b != expected {
		t.Fatalf("Expected %v, but got %v\n", expected, b)
	}
}

func TestSublistValidLiteralSubjects(t *testing.T) {
	checkBool(IsValidLiteralSubject("foo"), true, t)
	checkBool(IsValidLiteralSubject(".foo"), false, t)
	checkBool(IsValidLiteralSubject("foo."), false, t)
	checkBool(IsValidLiteralSubject("foo..bar"), false, t)
	checkBool(IsValidLiteralSubject("foo.bar.*"), false, t)
	checkBool(IsValidLiteralSubject("foo.bar.>"), false, t)
	checkBool(IsValidLiteralSubject("*"), false, t)
	checkBool(IsValidLiteralSubject(">"), false, t)
	// The followings have widlcards characters but are not
	// considered as such because they are not individual tokens.
	checkBool(IsValidLiteralSubject("foo*"), true, t)
	checkBool(IsValidLiteralSubject("foo**"), true, t)
	checkBool(IsValidLiteralSubject("foo.**"), true, t)
	checkBool(IsValidLiteralSubject("foo*bar"), true, t)
	checkBool(IsValidLiteralSubject("foo.*bar"), true, t)
	checkBool(IsValidLiteralSubject("foo*.bar"), true, t)
	checkBool(IsValidLiteralSubject("*bar"), true, t)
	checkBool(IsValidLiteralSubject("foo>"), true, t)
	checkBool(IsValidLiteralSubject("foo>>"), true, t)
	checkBool(IsValidLiteralSubject("foo.>>"), true, t)
	checkBool(IsValidLiteralSubject("foo>bar"), true, t)
	checkBool(IsValidLiteralSubject("foo.>bar"), true, t)
	checkBool(IsValidLiteralSubject("foo>.bar"), true, t)
	checkBool(IsValidLiteralSubject(">bar"), true, t)
}

func TestSublistValidlSubjects(t *testing.T) {
	checkBool(IsValidSubject("."), false, t)
	checkBool(IsValidSubject(".foo"), false, t)
	checkBool(IsValidSubject("foo."), false, t)
	checkBool(IsValidSubject("foo..bar"), false, t)
	checkBool(IsValidSubject(">.bar"), false, t)
	checkBool(IsValidSubject("foo.>.bar"), false, t)
	checkBool(IsValidSubject("foo"), true, t)
	checkBool(IsValidSubject("foo.bar.*"), true, t)
	checkBool(IsValidSubject("foo.bar.>"), true, t)
	checkBool(IsValidSubject("*"), true, t)
	checkBool(IsValidSubject(">"), true, t)
	checkBool(IsValidSubject("foo*"), true, t)
	checkBool(IsValidSubject("foo**"), true, t)
	checkBool(IsValidSubject("foo.**"), true, t)
	checkBool(IsValidSubject("foo*bar"), true, t)
	checkBool(IsValidSubject("foo.*bar"), true, t)
	checkBool(IsValidSubject("foo*.bar"), true, t)
	checkBool(IsValidSubject("*bar"), true, t)
	checkBool(IsValidSubject("foo>"), true, t)
	checkBool(IsValidSubject("foo.>>"), true, t)
	checkBool(IsValidSubject("foo>bar"), true, t)
	checkBool(IsValidSubject("foo.>bar"), true, t)
	checkBool(IsValidSubject("foo>.bar"), true, t)
	checkBool(IsValidSubject(">bar"), true, t)
}

func TestSublistMatchLiterals(t *testing.T) {
	checkBool(matchLiteral("foo", "foo"), true, t)
	checkBool(matchLiteral("foo", "bar"), false, t)
	checkBool(matchLiteral("foo", "*"), true, t)
	checkBool(matchLiteral("foo", ">"), true, t)
	checkBool(matchLiteral("foo.bar", ">"), true, t)
	checkBool(matchLiteral("foo.bar", "foo.>"), true, t)
	checkBool(matchLiteral("foo.bar", "bar.>"), false, t)
	checkBool(matchLiteral("stats.test.22", "stats.>"), true, t)
	checkBool(matchLiteral("stats.test.22", "stats.*.*"), true, t)
	checkBool(matchLiteral("foo.bar", "foo"), false, t)
	checkBool(matchLiteral("stats.test.foos", "stats.test.foos"), true, t)
	checkBool(matchLiteral("stats.test.foos", "stats.test.foo"), false, t)
	checkBool(matchLiteral("stats.test", "stats.test.*"), false, t)
	checkBool(matchLiteral("stats.test.foos", "stats.*"), false, t)
	checkBool(matchLiteral("stats.test.foos", "stats.*.*.foos"), false, t)

	// These are cases where wildcards characters should not be considered
	// wildcards since they do not follow the rules of wildcards.
	checkBool(matchLiteral("*bar", "*bar"), true, t)
	checkBool(matchLiteral("foo*", "foo*"), true, t)
	checkBool(matchLiteral("foo*bar", "foo*bar"), true, t)
	checkBool(matchLiteral("foo.***.bar", "foo.***.bar"), true, t)
	checkBool(matchLiteral(">bar", ">bar"), true, t)
	checkBool(matchLiteral("foo>", "foo>"), true, t)
	checkBool(matchLiteral("foo>bar", "foo>bar"), true, t)
	checkBool(matchLiteral("foo.>>>.bar", "foo.>>>.bar"), true, t)
}

func TestSubjectIsLiteral(t *testing.T) {
	checkBool(subjectIsLiteral("foo"), true, t)
	checkBool(subjectIsLiteral("foo.bar"), true, t)
	checkBool(subjectIsLiteral("foo*.bar"), true, t)
	checkBool(subjectIsLiteral("*"), false, t)
	checkBool(subjectIsLiteral(">"), false, t)
	checkBool(subjectIsLiteral("foo.*"), false, t)
	checkBool(subjectIsLiteral("foo.>"), false, t)
	checkBool(subjectIsLiteral("foo.*.>"), false, t)
	checkBool(subjectIsLiteral("foo.*.bar"), false, t)
	checkBool(subjectIsLiteral("foo.bar.>"), false, t)
}

func TestSublistBadSubjectOnRemove(t *testing.T) {
	testSublistBadSubjectOnRemove(t, NewSublistWithCache())
}

func TestSublistBadSubjectOnRemoveNoCache(t *testing.T) {
	testSublistBadSubjectOnRemove(t, NewSublistNoCache())
}

func testSublistBadSubjectOnRemove(t *testing.T, s *Sublist) {
	bad := "a.b..d"
	sub := newSub(bad)

	if err := s.Insert(sub); err != ErrInvalidSubject {
		t.Fatalf("Expected ErrInvalidSubject, got %v\n", err)
	}

	if err := s.Remove(sub); err != ErrInvalidSubject {
		t.Fatalf("Expected ErrInvalidSubject, got %v\n", err)
	}

	badfwc := "a.>.b"
	if err := s.Remove(newSub(badfwc)); err != ErrInvalidSubject {
		t.Fatalf("Expected ErrInvalidSubject, got %v\n", err)
	}
}

// This is from bug report #18
func TestSublistTwoTokenPubMatchSingleTokenSub(t *testing.T) {
	testSublistTwoTokenPubMatchSingleTokenSub(t, NewSublistWithCache())
}

func TestSublistTwoTokenPubMatchSingleTokenSubNoCache(t *testing.T) {
	testSublistTwoTokenPubMatchSingleTokenSub(t, NewSublistNoCache())
}

func testSublistTwoTokenPubMatchSingleTokenSub(t *testing.T, s *Sublist) {
	sub := newSub("foo")
	s.Insert(sub)
	r := s.Match("foo")
	verifyLen(r.psubs, 1, t)
	verifyMember(r.psubs, sub, t)
	r = s.Match("foo.bar")
	verifyLen(r.psubs, 0, t)
}

func TestSublistInsertWithWildcardsAsLiterals(t *testing.T) {
	testSublistInsertWithWildcardsAsLiterals(t, NewSublistWithCache())
}

func TestSublistInsertWithWildcardsAsLiteralsNoCache(t *testing.T) {
	testSublistInsertWithWildcardsAsLiterals(t, NewSublistNoCache())
}

func testSublistInsertWithWildcardsAsLiterals(t *testing.T, s *Sublist) {
	subjects := []string{"foo.*-", "foo.>-"}
	for _, subject := range subjects {
		sub := newSub(subject)
		s.Insert(sub)
		// Should find no match
		r := s.Match("foo.bar")
		verifyLen(r.psubs, 0, t)
		// Should find a match
		r = s.Match(subject)
		verifyLen(r.psubs, 1, t)
	}
}

func TestSublistRemoveWithWildcardsAsLiterals(t *testing.T) {
	testSublistRemoveWithWildcardsAsLiterals(t, NewSublistWithCache())
}

func TestSublistRemoveWithWildcardsAsLiteralsNoCache(t *testing.T) {
	testSublistRemoveWithWildcardsAsLiterals(t, NewSublistNoCache())
}

func testSublistRemoveWithWildcardsAsLiterals(t *testing.T, s *Sublist) {
	subjects := []string{"foo.*-", "foo.>-"}
	for _, subject := range subjects {
		sub := newSub(subject)
		s.Insert(sub)
		// Should find no match
		rsub := newSub("foo.bar")
		s.Remove(rsub)
		if c := s.Count(); c != 1 {
			t.Fatalf("Expected sublist to still contain sub, got %v", c)
		}
		s.Remove(sub)
		if c := s.Count(); c != 0 {
			t.Fatalf("Expected sublist to be empty, got %v", c)
		}
	}
}

func TestSublistRaceOnRemove(t *testing.T) {
	testSublistRaceOnRemove(t, NewSublistWithCache())
}

func TestSublistRaceOnRemoveNoCache(t *testing.T) {
	testSublistRaceOnRemove(t, NewSublistNoCache())
}

func testSublistRaceOnRemove(t *testing.T, s *Sublist) {
	var (
		total = 100
		subs  = make(map[int]*subscription, total) // use map for randomness
	)
	for i := 0; i < total; i++ {
		sub := newQSub("foo", "bar")
		subs[i] = sub
	}

	for i := 0; i < 2; i++ {
		for _, sub := range subs {
			s.Insert(sub)
		}
		// Call Match() once or twice, to make sure we get from cache
		if i == 1 {
			s.Match("foo")
		}
		// This will be from cache when i==1
		r := s.Match("foo")
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			for _, sub := range subs {
				s.Remove(sub)
			}
			wg.Done()
		}()
		for _, qsub := range r.qsubs {
			for i := 0; i < len(qsub); i++ {
				sub := qsub[i]
				if string(sub.queue) != "bar" {
					t.Fatalf("Queue name should be bar, got %s", qsub[i].queue)
				}
			}
		}
		wg.Wait()
	}

	// Repeat tests with regular subs
	for i := 0; i < total; i++ {
		sub := newSub("foo")
		subs[i] = sub
	}

	for i := 0; i < 2; i++ {
		for _, sub := range subs {
			s.Insert(sub)
		}
		// Call Match() once or twice, to make sure we get from cache
		if i == 1 {
			s.Match("foo")
		}
		// This will be from cache when i==1
		r := s.Match("foo")
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			for _, sub := range subs {
				s.Remove(sub)
			}
			wg.Done()
		}()
		for i := 0; i < len(r.psubs); i++ {
			sub := r.psubs[i]
			if string(sub.subject) != "foo" {
				t.Fatalf("Subject should be foo, got %s", sub.subject)
			}
		}
		wg.Wait()
	}
}

func TestSublistRaceOnInsert(t *testing.T) {
	testSublistRaceOnInsert(t, NewSublistWithCache())
}

func TestSublistRaceOnInsertNoCache(t *testing.T) {
	testSublistRaceOnInsert(t, NewSublistNoCache())
}

func testSublistRaceOnInsert(t *testing.T, s *Sublist) {
	var (
		total = 100
		subs  = make(map[int]*subscription, total) // use map for randomness
		wg    sync.WaitGroup
	)
	for i := 0; i < total; i++ {
		sub := newQSub("foo", "bar")
		subs[i] = sub
	}
	wg.Add(1)
	go func() {
		for _, sub := range subs {
			s.Insert(sub)
		}
		wg.Done()
	}()
	for i := 0; i < 1000; i++ {
		r := s.Match("foo")
		for _, qsubs := range r.qsubs {
			for _, qsub := range qsubs {
				if string(qsub.queue) != "bar" {
					t.Fatalf("Expected queue name to be bar, got %v", string(qsub.queue))
				}
			}
		}
	}
	wg.Wait()

	// Repeat the test with plain subs
	for i := 0; i < total; i++ {
		sub := newSub("foo")
		subs[i] = sub
	}
	wg.Add(1)
	go func() {
		for _, sub := range subs {
			s.Insert(sub)
		}
		wg.Done()
	}()
	for i := 0; i < 1000; i++ {
		r := s.Match("foo")
		for _, sub := range r.psubs {
			if string(sub.subject) != "foo" {
				t.Fatalf("Expected subject to be foo, got %v", string(sub.subject))
			}
		}
	}
	wg.Wait()
}

func TestSublistRaceOnMatch(t *testing.T) {
	s := NewSublistNoCache()
	s.Insert(newQSub("foo.*", "workers"))
	s.Insert(newQSub("foo.bar", "workers"))
	s.Insert(newSub("foo.*"))
	s.Insert(newSub("foo.bar"))

	wg := sync.WaitGroup{}
	wg.Add(2)
	errCh := make(chan error, 2)
	f := func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			r := s.Match("foo.bar")
			for _, sub := range r.psubs {
				if !strings.HasPrefix(string(sub.subject), "foo.") {
					errCh <- fmt.Errorf("Wrong subject: %s", sub.subject)
					return
				}
			}
			for _, qsub := range r.qsubs {
				for _, sub := range qsub {
					if string(sub.queue) != "workers" {
						errCh <- fmt.Errorf("Wrong queue name: %s", sub.queue)
						return
					}
				}
			}
		}
	}
	go f()
	go f()
	wg.Wait()
	select {
	case e := <-errCh:
		t.Fatalf(e.Error())
	default:
	}
}

// Remote subscriptions for queue subscribers will be weighted such that a single subscription
// is received, but represents all of the queue subscribers on the remote side.
func TestSublistRemoteQueueSubscriptions(t *testing.T) {
	testSublistRemoteQueueSubscriptions(t, NewSublistWithCache())
}

func TestSublistRemoteQueueSubscriptionsNoCache(t *testing.T) {
	testSublistRemoteQueueSubscriptions(t, NewSublistNoCache())
}

func testSublistRemoteQueueSubscriptions(t *testing.T, s *Sublist) {
	// Normals
	s1 := newQSub("foo", "bar")
	s2 := newQSub("foo", "bar")
	s.Insert(s1)
	s.Insert(s2)

	// Now do weighted remotes.
	rs1 := newRemoteQSub("foo", "bar", 10)
	s.Insert(rs1)
	rs2 := newRemoteQSub("foo", "bar", 10)
	s.Insert(rs2)

	// These are just shadowed in results, so should appear as 4 subs.
	verifyCount(s, 4, t)

	r := s.Match("foo")
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 1, t)
	verifyLen(r.qsubs[0], 22, t)

	s.Remove(s1)
	s.Remove(rs1)

	verifyCount(s, 2, t)

	// Now make sure our shadowed results are correct after a removal.
	r = s.Match("foo")
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 1, t)
	verifyLen(r.qsubs[0], 11, t)

	// Now do an update to an existing remote sub to update its weight.
	rs2.qw = 1
	s.UpdateRemoteQSub(rs2)

	// Results should reflect new weight.
	r = s.Match("foo")
	verifyLen(r.psubs, 0, t)
	verifyQLen(r.qsubs, 1, t)
	verifyLen(r.qsubs[0], 2, t)
}

func TestSublistSharedEmptyResult(t *testing.T) {
	s := NewSublistWithCache()
	r1 := s.Match("foo")
	verifyLen(r1.psubs, 0, t)
	verifyQLen(r1.qsubs, 0, t)

	r2 := s.Match("bar")
	verifyLen(r2.psubs, 0, t)
	verifyQLen(r2.qsubs, 0, t)

	if r1 != r2 {
		t.Fatalf("Expected empty result to be a shared result set")
	}
}

func TestSublistNoCacheStats(t *testing.T) {
	s := NewSublistNoCache()
	s.Insert(newSub("foo"))
	s.Insert(newSub("bar"))
	s.Insert(newSub("baz"))
	s.Insert(newSub("foo.bar.baz"))
	s.Match("a.b.c")
	s.Match("bar")
	stats := s.Stats()
	if stats.NumCache != 0 {
		t.Fatalf("Expected 0 for NumCache stat, got %d", stats.NumCache)
	}
}

func TestSublistAll(t *testing.T) {
	s := NewSublistNoCache()
	subs := []*subscription{
		newSub("foo.bar.baz"),
		newSub("foo"),
		newSub("baz"),
	}
	// alter client's kind
	subs[0].client.kind = LEAF
	for _, sub := range subs {
		s.Insert(sub)
	}

	var buf [32]*subscription
	output := buf[:0]
	s.All(&output)
	if len(output) != len(subs) {
		t.Fatalf("Expected %d for All, got %d", len(subs), len(output))
	}
}

// -- Benchmarks Setup --

var benchSublistSubs []*subscription
var benchSublistSl = NewSublistWithCache()

func init() {
	initSublist := false
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "test.bench" {
			initSublist = true
		}
	})
	if initSublist {
		benchSublistSubs = make([]*subscription, 0, 256*1024)
		toks := []string{"synadia", "nats", "jetstream", "nkeys", "jwt", "deny", "auth", "drain"}
		subsInit("", toks)
		for i := 0; i < len(benchSublistSubs); i++ {
			benchSublistSl.Insert(benchSublistSubs[i])
		}
		addWildcards()
	}
}

func subsInit(pre string, toks []string) {
	var sub string
	for _, t := range toks {
		if len(pre) > 0 {
			sub = pre + tsep + t
		} else {
			sub = t
		}
		benchSublistSubs = append(benchSublistSubs, newSub(sub))
		if len(strings.Split(sub, tsep)) < 5 {
			subsInit(sub, toks)
		}
	}
}

func addWildcards() {
	benchSublistSl.Insert(newSub("cloud.>"))
	benchSublistSl.Insert(newSub("cloud.nats.component.>"))
	benchSublistSl.Insert(newSub("cloud.*.*.nkeys.*"))
}

// -- Benchmarks Setup End --

func Benchmark______________________SublistInsert(b *testing.B) {
	s := NewSublistWithCache()
	for i, l := 0, len(benchSublistSubs); i < b.N; i++ {
		index := i % l
		s.Insert(benchSublistSubs[index])
	}
}

func Benchmark_______________SublistInsertNoCache(b *testing.B) {
	s := NewSublistNoCache()
	for i, l := 0, len(benchSublistSubs); i < b.N; i++ {
		index := i % l
		s.Insert(benchSublistSubs[index])
	}
}

func benchSublistTokens(b *testing.B, tokens string) {
	for i := 0; i < b.N; i++ {
		benchSublistSl.Match(tokens)
	}
}

func Benchmark____________SublistMatchSingleToken(b *testing.B) {
	benchSublistTokens(b, "synadia")
}

func Benchmark______________SublistMatchTwoTokens(b *testing.B) {
	benchSublistTokens(b, "synadia.nats")
}

func Benchmark____________SublistMatchThreeTokens(b *testing.B) {
	benchSublistTokens(b, "synadia.nats.jetstream")
}

func Benchmark_____________SublistMatchFourTokens(b *testing.B) {
	benchSublistTokens(b, "synadia.nats.jetstream.nkeys")
}

func Benchmark_SublistMatchFourTokensSingleResult(b *testing.B) {
	benchSublistTokens(b, "synadia.nats.jetstream.nkeys")
}

func Benchmark_SublistMatchFourTokensMultiResults(b *testing.B) {
	benchSublistTokens(b, "cloud.nats.component.router")
}

func Benchmark_______SublistMissOnLastTokenOfFive(b *testing.B) {
	benchSublistTokens(b, "synadia.nats.jetstream.nkeys.ZZZZ")
}

func multiRead(b *testing.B, num int) {
	var swg, fwg sync.WaitGroup
	swg.Add(num)
	fwg.Add(num)
	s := "synadia.nats.jetstream.nkeys"
	for i := 0; i < num; i++ {
		go func() {
			swg.Done()
			swg.Wait()
			n := b.N / num
			for i := 0; i < n; i++ {
				benchSublistSl.Match(s)
			}
			fwg.Done()
		}()
	}
	swg.Wait()
	b.ResetTimer()
	fwg.Wait()
}

func Benchmark____________Sublist10XMultipleReads(b *testing.B) {
	multiRead(b, 10)
}

func Benchmark___________Sublist100XMultipleReads(b *testing.B) {
	multiRead(b, 100)
}

func Benchmark__________Sublist1000XMultipleReads(b *testing.B) {
	multiRead(b, 1000)
}

func Benchmark________________SublistMatchLiteral(b *testing.B) {
	cachedSubj := "foo.foo.foo.foo.foo.foo.foo.foo.foo.foo"
	subjects := []string{
		"foo.foo.foo.foo.foo.foo.foo.foo.foo.foo",
		"foo.foo.foo.foo.foo.foo.foo.foo.foo.>",
		"foo.foo.foo.foo.foo.foo.foo.foo.>",
		"foo.foo.foo.foo.foo.foo.foo.>",
		"foo.foo.foo.foo.foo.foo.>",
		"foo.foo.foo.foo.foo.>",
		"foo.foo.foo.foo.>",
		"foo.foo.foo.>",
		"foo.foo.>",
		"foo.>",
		">",
		"foo.foo.foo.foo.foo.foo.foo.foo.foo.*",
		"foo.foo.foo.foo.foo.foo.foo.foo.*.*",
		"foo.foo.foo.foo.foo.foo.foo.*.*.*",
		"foo.foo.foo.foo.foo.foo.*.*.*.*",
		"foo.foo.foo.foo.foo.*.*.*.*.*",
		"foo.foo.foo.foo.*.*.*.*.*.*",
		"foo.foo.foo.*.*.*.*.*.*.*",
		"foo.foo.*.*.*.*.*.*.*.*",
		"foo.*.*.*.*.*.*.*.*.*",
		"*.*.*.*.*.*.*.*.*.*",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, subject := range subjects {
			if !matchLiteral(cachedSubj, subject) {
				b.Fatalf("Subject %q no match with %q", cachedSubj, subject)
			}
		}
	}
}

func Benchmark_____SublistMatch10kSubsWithNoCache(b *testing.B) {
	var nsubs = 512
	s := NewSublistNoCache()
	subject := "foo"
	for i := 0; i < nsubs; i++ {
		s.Insert(newSub(subject))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.Match(subject)
		if len(r.psubs) != nsubs {
			b.Fatalf("Results len is %d, should be %d", len(r.psubs), nsubs)
		}
	}
}

func removeTest(b *testing.B, singleSubject, doBatch bool, qgroup string) {
	s := NewSublistWithCache()
	subject := "foo"

	subs := make([]*subscription, 0, b.N)
	for i := 0; i < b.N; i++ {
		var sub *subscription
		if singleSubject {
			sub = newQSub(subject, qgroup)
		} else {
			sub = newQSub(fmt.Sprintf("%s.%d\n", subject, i), qgroup)
		}
		s.Insert(sub)
		subs = append(subs, sub)
	}

	// Actual test on Remove
	b.ResetTimer()
	if doBatch {
		s.RemoveBatch(subs)
	} else {
		for _, sub := range subs {
			s.Remove(sub)
		}
	}
}

func Benchmark__________SublistRemove1TokenSingle(b *testing.B) {
	removeTest(b, true, false, "")
}

func Benchmark___________SublistRemove1TokenBatch(b *testing.B) {
	removeTest(b, true, true, "")
}

func Benchmark_________SublistRemove2TokensSingle(b *testing.B) {
	removeTest(b, false, false, "")
}

func Benchmark__________SublistRemove2TokensBatch(b *testing.B) {
	removeTest(b, false, true, "")
}

func Benchmark________SublistRemove1TokenQGSingle(b *testing.B) {
	removeTest(b, true, false, "bar")
}

func Benchmark_________SublistRemove1TokenQGBatch(b *testing.B) {
	removeTest(b, true, true, "bar")
}

func removeMultiTest(b *testing.B, singleSubject, doBatch bool) {
	s := NewSublistWithCache()
	subject := "foo"
	var swg, fwg sync.WaitGroup
	swg.Add(b.N)
	fwg.Add(b.N)

	// We will have b.N go routines each with 1k subscriptions.
	sc := 1000

	for i := 0; i < b.N; i++ {
		go func() {
			subs := make([]*subscription, 0, sc)
			for n := 0; n < sc; n++ {
				var sub *subscription
				if singleSubject {
					sub = newSub(subject)
				} else {
					sub = newSub(fmt.Sprintf("%s.%d\n", subject, n))
				}
				s.Insert(sub)
				subs = append(subs, sub)
			}
			// Wait to start test
			swg.Done()
			swg.Wait()
			// Actual test on Remove
			if doBatch {
				s.RemoveBatch(subs)
			} else {
				for _, sub := range subs {
					s.Remove(sub)
				}
			}
			fwg.Done()
		}()
	}
	swg.Wait()
	b.ResetTimer()
	fwg.Wait()
}

// Check contention rates for remove from multiple Go routines.
// Reason for BatchRemove.
func Benchmark_________SublistRemove1kSingleMulti(b *testing.B) {
	removeMultiTest(b, true, false)
}

// Batch version
func Benchmark__________SublistRemove1kBatchMulti(b *testing.B) {
	removeMultiTest(b, true, true)
}

func Benchmark__SublistRemove1kSingle2TokensMulti(b *testing.B) {
	removeMultiTest(b, false, false)
}

// Batch version
func Benchmark___SublistRemove1kBatch2TokensMulti(b *testing.B) {
	removeMultiTest(b, false, true)
}

// Cache contention tests
func cacheContentionTest(b *testing.B, numMatchers, numAdders, numRemovers int) {
	var swg, fwg, mwg sync.WaitGroup
	total := numMatchers + numAdders + numRemovers
	swg.Add(total)
	fwg.Add(total)
	mwg.Add(numMatchers)

	mu := sync.RWMutex{}
	subs := make([]*subscription, 0, 8192)

	quitCh := make(chan struct{})

	// Set up a new sublist. subjects will be foo.bar.baz.N
	s := NewSublistWithCache()
	mu.Lock()
	for i := 0; i < 10000; i++ {
		sub := newSub(fmt.Sprintf("foo.bar.baz.%d", i))
		s.Insert(sub)
		subs = append(subs, sub)
	}
	mu.Unlock()

	// Now warm up the cache
	for i := 0; i < slCacheMax; i++ {
		s.Match(fmt.Sprintf("foo.bar.baz.%d", i))
	}

	// Setup go routines.

	// Adders
	for i := 0; i < numAdders; i++ {
		go func() {
			swg.Done()
			swg.Wait()
			for {
				select {
				case <-quitCh:
					fwg.Done()
					return
				default:
					mu.Lock()
					next := len(subs)
					subj := "foo.bar.baz." + strconv.FormatInt(int64(next), 10)
					sub := newSub(subj)
					subs = append(subs, sub)
					mu.Unlock()
					s.Insert(sub)
				}
			}
		}()
	}

	// Removers
	for i := 0; i < numRemovers; i++ {
		go func() {
			prand := rand.New(rand.NewSource(time.Now().UnixNano()))
			swg.Done()
			swg.Wait()
			for {
				select {
				case <-quitCh:
					fwg.Done()
					return
				default:
					mu.RLock()
					lh := len(subs) - 1
					index := prand.Intn(lh)
					sub := subs[index]
					mu.RUnlock()
					s.Remove(sub)
				}
			}
		}()
	}

	// Matchers
	for i := 0; i < numMatchers; i++ {
		go func() {
			id := nuid.New()
			swg.Done()
			swg.Wait()

			// We will miss on purpose to blow the cache.
			n := b.N / numMatchers
			for i := 0; i < n; i++ {
				subj := "foo.bar.baz." + id.Next()
				s.Match(subj)
			}
			mwg.Done()
			fwg.Done()
		}()
	}

	swg.Wait()
	b.ResetTimer()
	mwg.Wait()
	b.StopTimer()
	close(quitCh)
	fwg.Wait()
}

func Benchmark____SublistCacheContention10M10A10R(b *testing.B) {
	cacheContentionTest(b, 10, 10, 10)
}

func Benchmark_SublistCacheContention100M100A100R(b *testing.B) {
	cacheContentionTest(b, 100, 100, 100)
}

func Benchmark____SublistCacheContention1kM1kA1kR(b *testing.B) {
	cacheContentionTest(b, 1024, 1024, 1024)
}

func Benchmark_SublistCacheContention10kM10kA10kR(b *testing.B) {
	cacheContentionTest(b, 10*1024, 10*1024, 10*1024)
}

func Benchmark______________IsValidLiteralSubject(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsValidLiteralSubject("foo.bar.baz.22")
	}
}

func Benchmark___________________subjectIsLiteral(b *testing.B) {
	for i := 0; i < b.N; i++ {
		subjectIsLiteral("foo.bar.baz.22")
	}
}