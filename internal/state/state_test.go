package state

import (
	"testing"
	"time"
)

func at(seconds int64) time.Time {
	return time.Unix(seconds, 0).UTC()
}

func TestRecord_StaysUpOnSuccess(t *testing.T) {
	var s CheckState
	tr := s.Record(true, 3, at(0))
	if tr.Kind != NoTransition {
		t.Fatalf("expected no transition on first success, got %v", tr.Kind)
	}
	if s.Current != StatusUp {
		t.Fatalf("expected status Up, got %v", s.Current)
	}
}

func TestRecord_DoesNotAlertBelowThreshold(t *testing.T) {
	var s CheckState
	s.Record(true, 3, at(0)) // establish Up

	tr := s.Record(false, 3, at(1))
	if tr.Kind != NoTransition {
		t.Fatalf("expected no transition on 1st failure below threshold, got %v", tr.Kind)
	}
	if s.Current != StatusUp {
		t.Fatalf("expected to remain Up below threshold, got %v", s.Current)
	}

	tr2 := s.Record(false, 3, at(2))
	if tr2.Kind != NoTransition {
		t.Fatalf("expected no transition on 2nd failure below threshold, got %v", tr2.Kind)
	}
	if s.Current != StatusUp {
		t.Fatalf("expected to remain Up below threshold, got %v", s.Current)
	}
}

func TestRecord_TransitionsToDownAtThreshold(t *testing.T) {
	var s CheckState
	s.Record(true, 3, at(0))
	s.Record(false, 3, at(1))
	s.Record(false, 3, at(2))
	tr := s.Record(false, 3, at(3))

	if tr.Kind != TransitionToDown {
		t.Fatalf("expected TransitionToDown at 3rd consecutive failure, got %v", tr.Kind)
	}
	if tr.ConsecutiveFails != 3 {
		t.Errorf("expected ConsecutiveFails=3, got %d", tr.ConsecutiveFails)
	}
	if s.Current != StatusDown {
		t.Fatalf("expected status Down, got %v", s.Current)
	}
	if !s.DownSince.Equal(at(3)) {
		t.Errorf("expected DownSince=%v, got %v", at(3), s.DownSince)
	}
}

func TestRecord_NoRepeatedAlertWhileFlapping(t *testing.T) {
	var s CheckState
	s.Record(true, 2, at(0))
	s.Record(false, 2, at(1))
	tr := s.Record(false, 2, at(2)) // crosses threshold
	if tr.Kind != TransitionToDown {
		t.Fatalf("expected TransitionToDown, got %v", tr.Kind)
	}

	// Further failures while already down must NOT re-fire the alert.
	for i := int64(3); i < 10; i++ {
		tr := s.Record(false, 2, at(i))
		if tr.Kind != NoTransition {
			t.Fatalf("expected NoTransition while already down (t=%d), got %v", i, tr.Kind)
		}
	}
}

func TestRecord_RecoversAndReportsDowntime(t *testing.T) {
	var s CheckState
	s.Record(true, 2, at(0))
	s.Record(false, 2, at(10))
	s.Record(false, 2, at(20)) // down at t=20

	tr := s.Record(true, 2, at(50)) // recovers at t=50
	if tr.Kind != TransitionToUp {
		t.Fatalf("expected TransitionToUp, got %v", tr.Kind)
	}
	if tr.Downtime != 30*time.Second {
		t.Errorf("expected downtime of 30s (50-20), got %s", tr.Downtime)
	}
	if s.Current != StatusUp {
		t.Fatalf("expected status Up after recovery, got %v", s.Current)
	}
}

func TestRecord_FlappingBelowThresholdNeverAlerts(t *testing.T) {
	// Alternating single failures and successes should never cross a
	// threshold of 3, and should never report a transition.
	var s CheckState
	s.Record(true, 3, at(0))
	for i := int64(1); i < 20; i++ {
		var tr Transition
		if i%2 == 0 {
			tr = s.Record(true, 3, at(i))
		} else {
			tr = s.Record(false, 3, at(i))
		}
		if tr.Kind != NoTransition {
			t.Fatalf("flapping below threshold should never alert (t=%d), got %v", i, tr.Kind)
		}
	}
	if s.Current != StatusUp {
		t.Fatalf("expected to remain Up throughout, got %v", s.Current)
	}
}

func TestRecord_ResetsFailCountOnIntermittentSuccess(t *testing.T) {
	var s CheckState
	s.Record(true, 3, at(0))
	s.Record(false, 3, at(1))
	s.Record(false, 3, at(2))
	// A success resets the streak before it would have crossed threshold.
	s.Record(true, 3, at(3))
	s.Record(false, 3, at(4))
	tr := s.Record(false, 3, at(5))
	if tr.Kind != NoTransition {
		t.Fatalf("expected no transition: fail streak should have reset, got %v", tr.Kind)
	}
	if s.Current != StatusUp {
		t.Fatalf("expected still Up, got %v", s.Current)
	}
}

func TestRecord_FailThresholdOfOneAlertsImmediately(t *testing.T) {
	var s CheckState
	s.Record(true, 1, at(0))
	tr := s.Record(false, 1, at(1))
	if tr.Kind != TransitionToDown {
		t.Fatalf("expected immediate TransitionToDown with fail_threshold=1, got %v", tr.Kind)
	}
}

func TestRecord_UnknownToDownRequiresThreshold(t *testing.T) {
	// Starting from StatusUnknown (never seen a success), failures should
	// still require the threshold before alerting.
	var s CheckState
	tr1 := s.Record(false, 2, at(0))
	if tr1.Kind != NoTransition {
		t.Fatalf("expected no transition on first ever failure below threshold, got %v", tr1.Kind)
	}
	tr2 := s.Record(false, 2, at(1))
	if tr2.Kind != TransitionToDown {
		t.Fatalf("expected TransitionToDown at threshold from Unknown, got %v", tr2.Kind)
	}
}

func TestShouldWarnSSL_WithinWindowOncePerDay(t *testing.T) {
	var s CheckState
	expiry := at(0).Add(5 * 24 * time.Hour) // expires in 5 days

	now := at(0)
	if !s.ShouldWarnSSL(expiry, 14, now) {
		t.Fatal("expected a warning: within the 14 day window")
	}
	// Same day, later time: should not warn again.
	sameDayLater := now.Add(3 * time.Hour)
	if s.ShouldWarnSSL(expiry, 14, sameDayLater) {
		t.Fatal("expected no repeat warning on the same day")
	}
	// Next day: should warn again.
	nextDay := now.Add(24 * time.Hour)
	if !s.ShouldWarnSSL(expiry, 14, nextDay) {
		t.Fatal("expected a new warning on the next day")
	}
}

func TestShouldWarnSSL_OutsideWindow(t *testing.T) {
	var s CheckState
	expiry := at(0).Add(60 * 24 * time.Hour) // 60 days out
	if s.ShouldWarnSSL(expiry, 14, at(0)) {
		t.Fatal("expected no warning: expiry is well outside the warn window")
	}
}

func TestShouldWarnSSL_DisabledWhenWarnDaysZero(t *testing.T) {
	var s CheckState
	expiry := at(0).Add(1 * time.Hour)
	if s.ShouldWarnSSL(expiry, 0, at(0)) {
		t.Fatal("expected no warning when ssl_warn_days is 0 (disabled)")
	}
}

func TestStatus_String(t *testing.T) {
	cases := map[Status]string{
		StatusUnknown: "UNKNOWN",
		StatusUp:      "UP",
		StatusDown:    "DOWN",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", status, got, want)
		}
	}
}
