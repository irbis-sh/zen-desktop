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

	t.Run("'@@||page$important' should cancel '||page$important'", func(t *testing.T) {
		t.Parallel()

		filterName := "test"

		er := &ExceptionRule{
			Rule: rule.Rule{
				RawRule:    "@@||example.com$important",
				FilterName: &filterName,
			},
		}
		r := &rule.Rule{
			RawRule:    "||example.com$important",
			FilterName: &filterName,
		}
		r.ParseModifiers([]string{"important"})
		er.ParseModifiers([]string{"important"})

		want := true
		if got := er.Cancels(r); got != want {
			t.Errorf("'%s'.Cancels('%s') = %t, want %t", er.RawRule, r.RawRule, got, want)
		}
	})

	t.Run("'@@||page' should not cancel '||page$important'", func(t *testing.T) {
		t.Parallel()

		filterName := "test"

		er := &ExceptionRule{
			Rule: rule.Rule{
				RawRule:    "@@||example.com",
				FilterName: &filterName,
			},
		}
		r := &rule.Rule{
			RawRule:    "||example.com$important",
			FilterName: &filterName,
		}
		r.ParseModifiers([]string{"important"})

		want := false
		if got := er.Cancels(r); got != want {
			t.Errorf("'%s'.Cancels('%s') = %t, want %t", er.RawRule, r.RawRule, got, want)
		}
	})

	t.Run("'@@||page$important' should cancel '||page'", func(t *testing.T) {
		t.Parallel()

		filterName := "test"

		er := &ExceptionRule{
			Rule: rule.Rule{
				RawRule:    "@@||example.com$important",
				FilterName: &filterName,
			},
		}
		r := &rule.Rule{
			RawRule:    "||example.com",
			FilterName: &filterName,
		}
		er.ParseModifiers([]string{"important"})

		want := true
		if got := er.Cancels(r); got != want {
			t.Errorf("'%s'.Cancels('%s') = %t, want %t", er.RawRule, r.RawRule, got, want)
		}
	})
}
