package exceptionrule

import (
	"fmt"
	"testing"

	"github.com/irbis-sh/zen-desktop/internal/networkrules/rule"
)

func TestExceptionRule(t *testing.T) {
	t.Parallel()

	t.Run("'@@||page' should cancel '||page$document'", func(t *testing.T) {
		t.Parallel()

		filterName := "test"

		er := &ExceptionRule{
			Rule: rule.Rule{
				RawRule:    "||example.com",
				FilterName: &filterName,
			},
		}
		r := &rule.Rule{
			RawRule:    "||example.com$document",
			FilterName: &filterName,
		}
		r.ParseModifiers([]string{"document"})

		want := true
		if got := er.Cancels(r); got != want {
			t.Errorf("'%s'.Cancels('%s') = %t, want %t", er.RawRule, r.RawRule, got, want)
		}
	})

	t.Run("'@@||page$document' should cancel '||page$document'", func(t *testing.T) {
		t.Parallel()

		filterName := "test"

		er := &ExceptionRule{
			Rule: rule.Rule{
				RawRule:    "||example.com$document",
				FilterName: &filterName,
			},
		}
		r := &rule.Rule{
			RawRule:    "||example.com$document",
			FilterName: &filterName,
		}
		r.ParseModifiers([]string{"document"})
		er.ParseModifiers([]string{"document"})

		want := true
		if got := er.Cancels(r); got != want {
			t.Errorf("'%s'.Cancels('%s') = %t, want %t", er.RawRule, r.RawRule, got, want)
		}
	})

	t.Run("'@@||page$document' should not cancel '||page'", func(t *testing.T) {
		t.Parallel()

		filterName := "test"

		er := &ExceptionRule{
			Rule: rule.Rule{
				RawRule:    "||example.com^$document",
				FilterName: &filterName,
			},
		}
		r := &rule.Rule{
			RawRule:    "||example.com",
			FilterName: &filterName,
		}
		er.ParseModifiers([]string{"document"})

		want := false
		if got := er.Cancels(r); got != want {
			fmt.Println(got, want)
			t.Errorf("'%s'.Cancels('%s') = %t, want %t", er.RawRule, r.RawRule, got, want)
		}
	})

	t.Run("important exception should cancel important block", func(t *testing.T) {
		t.Parallel()

		er := &ExceptionRule{Rule: rule.Rule{Important: true}}
		r := &rule.Rule{Important: true}

		if !er.Cancels(r) {
			t.Errorf("Important exception rule should cancel important block rule")
		}
	})

	t.Run("normal exception should not cancel important block", func(t *testing.T) {
		t.Parallel()

		er := &ExceptionRule{Rule: rule.Rule{Important: false}}
		r := &rule.Rule{Important: true}

		if er.Cancels(r) {
			t.Errorf("Normal exception rule should NOT cancel important block rule")
		}
	})

	t.Run("important exception should cancel normal block", func(t *testing.T) {
		t.Parallel()

		er := &ExceptionRule{Rule: rule.Rule{Important: true}}
		r := &rule.Rule{Important: false}

		if !er.Cancels(r) {
			t.Errorf("Important exception rule should cancel normal block rule")
		}
	})
}
