package rule

import "testing"

func TestModifierInterner(t *testing.T) {
	t.Parallel()

	t.Run("shares the block and flags between rules", func(t *testing.T) {
		t.Parallel()

		in := NewModifierInterner()
		first, second := &Rule{}, &Rule{}
		for _, r := range []*Rule{first, second} {
			if err := in.Parse(r, "image,important"); err != nil {
				t.Fatal(err)
			}
		}

		if second.mods != first.mods {
			t.Error("second rule got a separate block, want the shared one")
		}
		if !second.Important() {
			t.Error("second rule Important() = false, want true")
		}
	})

	t.Run("does not share a failed parse", func(t *testing.T) {
		t.Parallel()

		// A shared nil block would turn the second rule into an unconditional block rule.
		in := NewModifierInterner()
		for i := range 2 {
			if err := in.Parse(&Rule{}, "image,popup"); err == nil {
				t.Errorf("Parse() call %d = nil, want error", i+1)
			}
		}
	})

	t.Run("nil interner parses without sharing", func(t *testing.T) {
		t.Parallel()

		var in *ModifierInterner
		r := &Rule{}
		if err := in.Parse(r, "document"); err != nil {
			t.Fatal(err)
		}
		if !r.BlocksNavigations() {
			t.Error("BlocksNavigations() = false, want true")
		}
	})
}
