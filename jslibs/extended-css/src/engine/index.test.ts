import { describe, test, beforeAll, beforeEach, afterAll, afterEach, expect } from '@jest/globals';

import { Engine } from '.';

describe.each([
  ['native :has/:is/:not support ON', true],
  ['native :has/:is/:not support OFF', false],
])('Engine (%s)', (_, nativeOn) => {
  let origCSS: typeof window.CSS;

  beforeAll(() => {
    origCSS = window.CSS;
    window.CSS = {
      supports: () => nativeOn,
      escape: (s: string) => s,
    } as unknown as typeof CSS;
  });

  afterAll(() => {
    window.CSS = origCSS;
  });

  let originalBody: string;
  let startedEngines: Engine[] = [];

  beforeEach(() => {
    originalBody = document.body.innerHTML;
  });

  afterEach(() => {
    // Stop all engines to disconnect MutationObservers and event handlers
    for (const engine of startedEngines) {
      engine.stop();
    }
    startedEngines = [];
    jest.useRealTimers();
    document.body.innerHTML = originalBody;
  });

  // Helper function to create test DOM structure
  const createTestDOM = (html: string) => {
    document.body.innerHTML = html;
  };

  // Helper function to check if element is hidden
  const isElementHidden = (element: Element): boolean => {
    const style = getComputedStyle(element);
    return style.display === 'none';
  };

  // Helper function to get visible elements by selector
  const getVisibleElements = (selector: string): Element[] => {
    return Array.from(document.documentElement.querySelectorAll(selector)).filter((el) => !isElementHidden(el));
  };

  const startEngine = (rules: string): Engine => {
    const engine = new Engine(rules);
    engine.start();
    startedEngines.push(engine);
    return engine;
  };

  describe('basic selector parsing and execution', () => {
    test('hides elements matching simple class selector', () => {
      createTestDOM(`
        <div class="hide-me">Should be hidden</div>
        <div class="keep-me">Should remain visible</div>
        <span class="hide-me">Should also be hidden</span>
      `);

      startEngine('.hide-me');

      expect(getVisibleElements('.hide-me')).toHaveLength(0);
      expect(getVisibleElements('.keep-me')).toHaveLength(1);
    });

    test('hides elements matching ID selector', () => {
      createTestDOM(`
        <div id="target">Should be hidden</div>
        <div id="other">Should remain visible</div>
      `);

      startEngine('#target');

      expect(getVisibleElements('#target')).toHaveLength(0);
      expect(getVisibleElements('#other')).toHaveLength(1);
    });

    test('hides elements matching tag selector', () => {
      createTestDOM(`
        <span>Should be hidden</span>
        <div>Should remain visible</div>
        <span>Should also be hidden</span>
      `);

      startEngine('span');

      expect(getVisibleElements('span')).toHaveLength(0);
      expect(getVisibleElements('div')).toHaveLength(1);
    });

    test('hides elements matching attribute selector', () => {
      createTestDOM(`
        <div data-ad="true">Should be hidden</div>
        <div data-content="true">Should remain visible</div>
        <span data-ad="banner">Should also be hidden</span>
      `);

      startEngine('[data-ad]');

      expect(getVisibleElements('[data-ad]')).toHaveLength(0);
      expect(getVisibleElements('[data-content]')).toHaveLength(1);
    });
  });

  describe(':has() pseudo-class functionality', () => {
    test('hides parent elements containing specific children', () => {
      createTestDOM(`
        <div id="container1">
          <span class="ad-marker">Advertisement</span>
          <p>Some content</p>
        </div>
        <div id="container2">
          <p>Clean content</p>
        </div>
        <div id="container3">
          <div class="ad-marker">Another ad</div>
        </div>
      `);

      startEngine('div:has(.ad-marker)');

      expect(getVisibleElements('#container1')).toHaveLength(0);
      expect(getVisibleElements('#container2')).toHaveLength(1);
      expect(getVisibleElements('#container3')).toHaveLength(0);
    });

    test('handles :has() with direct child combinator', () => {
      createTestDOM(`
        <div id="direct">
          <span class="marker">Direct child</span>
        </div>
        <div id="nested">
          <div>
            <span class="marker">Nested child</span>
          </div>
        </div>
      `);

      startEngine('div:has(> .marker)');

      expect(getVisibleElements('#direct')).toHaveLength(0);
      expect(getVisibleElements('#nested')).toHaveLength(1);
    });

    test('handles :has() with selector list (OR semantics)', () => {
      createTestDOM(`
        <div id="hasSpan"><span>Has span</span></div>
        <div id="hasP"><p class="marker">Has p.marker</p></div>
        <div id="hasBoth">
          <span>Has span</span>
          <p class="marker">Has p.marker</p>
        </div>
        <div id="hasNeither">Has neither</div>
      `);

      startEngine('div:has(span, .marker)');

      expect(getVisibleElements('#hasSpan')).toHaveLength(0);
      expect(getVisibleElements('#hasP')).toHaveLength(0);
      expect(getVisibleElements('#hasBoth')).toHaveLength(0);
      expect(getVisibleElements('#hasNeither')).toHaveLength(1);
    });
  });

  describe(':is() pseudo-class functionality', () => {
    test('hides elements matching any selector in list', () => {
      createTestDOM(`
        <div class="target">Should be hidden</div>
        <span id="special">Should be hidden</span>
        <p class="safe">Should remain visible</p>
        <div id="other">Should remain visible</div>
      `);

      startEngine(':is(.target, #special)');

      expect(getVisibleElements('.target')).toHaveLength(0);
      expect(getVisibleElements('#special')).toHaveLength(0);
      expect(getVisibleElements('.safe')).toHaveLength(1);
      expect(getVisibleElements('#other')).toHaveLength(1);
    });

    test('handles :is() with complex selectors', () => {
      createTestDOM(`
        <div class="container">
          <span class="item first">Should be hidden</span>
          <span class="item">Should remain visible</span>
          <p class="item last">Should be hidden</p>
        </div>
      `);

      startEngine('.container :is(.first, .last)');

      expect(getVisibleElements('.first')).toHaveLength(0);
      expect(getVisibleElements('.last')).toHaveLength(0);
      expect(getVisibleElements('.item:not(.first):not(.last)')).toHaveLength(1);
    });
  });

  describe('multiple rules and complex scenarios', () => {
    test('applies multiple rules independently', () => {
      createTestDOM(`
        <div class="ad">Ad content</div>
        <span class="tracker">Tracking pixel</span>
        <p class="content">Good content</p>
        <div class="popup">Popup</div>
      `);

      const rules = `
        .ad
        .tracker
        .popup
      `;

      startEngine(rules);

      expect(getVisibleElements('.ad')).toHaveLength(0);
      expect(getVisibleElements('.tracker')).toHaveLength(0);
      expect(getVisibleElements('.popup')).toHaveLength(0);
      expect(getVisibleElements('.content')).toHaveLength(1);
    });

    test('handles nested :has() and :is() selectors', () => {
      createTestDOM(`
        <div id="complex1" class="container">
          <div class="ad-wrapper">
            <span class="ad">Advertisement</span>
          </div>
        </div>
        <div id="complex2" class="container">
          <div class="content-wrapper">
            <span class="content">Clean content</span>
          </div>
        </div>
      `);

      startEngine(':is(.container:has(.ad))');

      expect(getVisibleElements('#complex1')).toHaveLength(0);
      expect(getVisibleElements('#complex2')).toHaveLength(1);
    });

    test('universal selector', () => {
      createTestDOM(`
        <div>Should be hidden</div>
        <span>Should also be hidden</span>
        <h3>Should also be hidden></h3>
        <p>
          <span>Should also be hidden</span>
          Should also be hidden
        </p>
      `);

      startEngine('*');

      expect(getVisibleElements('*')).toHaveLength(0);
    });

    test('pseudo-class followed by a combinator followed by a raw selector', () => {
      createTestDOM(`
        <div>Text</div>
        <span class="should-be-hidden"></span>
        <div>Text</div>
        <span class="should-be-hidden"></span>
        <span>Should not be hidden</span>
      `);

      startEngine('div:min-text-length(2) + span');

      expect(getVisibleElements('div')).toHaveLength(2);
      expect(getVisibleElements('.should-be-hidden')).toHaveLength(0);
      expect(getVisibleElements('span:not(.should-be-hidden)')).toHaveLength(1);
    });

    test('descendant elements with descendant combinator and universal selector', () => {
      createTestDOM(`
        <div id="parent">
          <span>Should be hidden (direct child)</span>
          <p>
            <a href="#">Should be hidden (nested)</a>
            <em>Should be hidden (nested)</em>
          </p>
          <ul>
            <li>Should be hidden (nested)</li>
          </ul>
        </div>
        <span>Should remain visible (not in div)</span>
      `);

      startEngine('#parent:min-text-length(2) *');

      expect(getVisibleElements('#parent')).toHaveLength(1);
      expect(getVisibleElements('#parent *')).toHaveLength(0);
      expect(getVisibleElements('span:not(#parent *)')).toHaveLength(1);
    });

    test('hides subsequent siblings with general sibling combinator and universal selector', () => {
      createTestDOM(`
        <span id="first">Should remain visible (before div)</span>
        <div id="reference">Reference div</div>
        <p>Should be hidden (after div)</p>
        <span>Should be hidden (after div)</span>
        <div id="another">
          Another Reference
          <span>Should remain visible (child of another div)</span>
        </div>
        <ul>Should be hidden (after div)</ul>
      `);

      startEngine('div:contains(Reference) ~ *');

      expect(getVisibleElements('#reference')).toHaveLength(1);
      expect(getVisibleElements('#first')).toHaveLength(1);
      expect(getVisibleElements('div ~ *')).toHaveLength(0);
      expect(getVisibleElements('#another span')).toHaveLength(1);
    });

    test('handles complex multi-step selector', () => {
      createTestDOM(`
        <section>
          <p>Text with more than 2 chars</p>
          <div id="parent-div">
            <div class="cls">Target parent</div>
            <span id="target1">Should be hidden</span>
            <span>Not adjacent, should remain visible</span>
          </div>
          <div>
            <div class="cls">Another target parent</div>
            <span id="target2">Should also be hidden</span>
          </div>
          <span>Not in the pattern, should remain visible</span>
          <p>s</p>
          <div>
            <div class="cls">Not after text with min-length, should remain visible</div>
            <span id="not-target">Should remain visible</span>
          </div>
        </section>
      `);

      startEngine(':min-text-length(2) + div > div:is(.cls) + span');

      expect(getVisibleElements('#target1')).toHaveLength(0);
      expect(getVisibleElements('#target2')).toHaveLength(0);
      expect(getVisibleElements('#not-target')).toHaveLength(1);
      expect(getVisibleElements('span:not(#target1):not(#target2)')).toHaveLength(3);
      expect(getVisibleElements('.cls')).toHaveLength(document.querySelectorAll('.cls').length);
    });

    test('"unhides" elements after they no longer match the selector', async () => {
      jest.useFakeTimers();

      createTestDOM(`
        <div class="dynamic"><div class="ad"></div></div>
        <div class="static">Should remain visible</div>
      `);

      startEngine('div:has(.ad)');

      expect(getVisibleElements('.dynamic')).toHaveLength(0);
      expect(getVisibleElements('.static')).toHaveLength(1);

      const ad = document.querySelector('.ad')!;
      ad.remove();

      await jest.runAllTimersAsync();

      expect(getVisibleElements('.dynamic')).toHaveLength(1);
      expect(getVisibleElements('.static')).toHaveLength(1);
      jest.useRealTimers();
    });

    test('handles dynamic content updates', () => {
      jest.useFakeTimers();
      createTestDOM(`
        <div class="dangerous">Ad</div>
      `);

      startEngine('div:has-text(Ad)');

      expect(getVisibleElements('div')).toHaveLength(0);

      for (let i = 0; i < 100; i++) {
        const div = document.createElement('div');
        div.textContent = 'Ad ' + i;
        document.body.appendChild(div);
      }

      jest.runAllTimers();

      expect(getVisibleElements('.dangerous')).toHaveLength(0);
      jest.useRealTimers();
    });
  });

  describe('edge cases and error handling', () => {
    test('handles empty rules gracefully', () => {
      createTestDOM(`
        <div class="test">Should remain visible</div>
      `);

      expect(() => startEngine('')).not.toThrow();
      expect(getVisibleElements('.test')).toHaveLength(1);
    });

    test('handles invalid CSS syntax gracefully', () => {
      createTestDOM(`
        <div class="test">Should remain visible</div>
      `);

      expect(() => startEngine('~~~~invalid css syntax~~~~~')).not.toThrow();
      expect(getVisibleElements('.test')).toHaveLength(1);
    });

    test('handles malformed selectors in forgiving mode', () => {
      createTestDOM(`
        <div class="valid">Should be hidden</div>
        <div class="other">Should remain visible</div>
      `);

      const rules = `
        .valid
        :invalid-pseudo
      `;

      expect(() => startEngine(rules)).not.toThrow();
      expect(getVisibleElements('.valid')).toHaveLength(0);
      expect(getVisibleElements('.other')).toHaveLength(1);
    });

    test('handles deeply nested structures', () => {
      const createNestedStructure = (depth: number): string => {
        if (depth === 0) return '<span class="deep-target">Deep content</span>';
        return `<div class="level-${depth}">${createNestedStructure(depth - 1)}</div>`;
      };

      createTestDOM(createNestedStructure(100));

      startEngine('div:has(.deep-target)');

      expect(getVisibleElements('.level-1')).toHaveLength(0);
      expect(document.querySelectorAll('.deep-target')).toHaveLength(1);
    });
  });

  describe('performance and optimization', () => {
    test('handles large number of elements', () => {
      const elements = Array.from(
        { length: 10000 },
        (_, i) => `<div class="${i % 2 === 0 ? 'even' : 'odd'}" id="item-${i}">Item ${i}</div>`,
      ).join('');

      createTestDOM(elements);

      startEngine('.even:has-text(Item)');

      expect(getVisibleElements('.even')).toHaveLength(0);
      expect(getVisibleElements('.odd')).toHaveLength(5000);
    });

    test('handles multiple engine instances independently', () => {
      createTestDOM(`
        <div class="target1">Target 1</div>
        <div class="target2">Target 2</div>
        <div class="safe">Safe content</div>
      `);

      startEngine('.target1');

      expect(getVisibleElements('.target1')).toHaveLength(0);
      expect(getVisibleElements('.target2')).toHaveLength(1);

      startEngine('.target2');

      expect(getVisibleElements('.target1')).toHaveLength(0);
      expect(getVisibleElements('.target2')).toHaveLength(0);
      expect(getVisibleElements('.safe')).toHaveLength(1);
    });
  });

  describe('real-world use cases', () => {
    test('blocks common ad patterns', () => {
      createTestDOM(`
        <div class="advertisement">Ad banner</div>
        <div data-ad-type="banner">Another ad</div>
        <div class="content">
          <div class="ad-container">
            <span class="ad-label">Sponsored</span>
            <div class="ad-content">Ad content</div>
          </div>
        </div>
        <article class="post">Clean content</article>
      `);

      const rules = `
        .advertisement
        [data-ad-type]
        div:has(.ad-label)
      `;

      startEngine(rules);

      expect(getVisibleElements('.advertisement')).toHaveLength(0);
      expect(getVisibleElements('[data-ad-type]')).toHaveLength(0);
      expect(getVisibleElements('.ad-container')).toHaveLength(0);
      expect(getVisibleElements('.post')).toHaveLength(1);
    });

    test('handles social media widgets', () => {
      createTestDOM(`
        <div class="social-widget" data-platform="facebook">
          <iframe src="https://facebook.com/plugins/panopticon"></iframe>
        </div>
        <div class="social-widget" data-platform="twitter">
          <iframe src="https://twitter.com/plugins/panopticon"></iframe>
        </div>
        <div class="content">
          <p>Article content</p>
        </div>
      `);

      startEngine('.social-widget:has(iframe[src])');

      expect(getVisibleElements('.social-widget')).toHaveLength(0);
      expect(getVisibleElements('.content')).toHaveLength(1);
    });

    test('hides elements using :matches-attr', () => {
      createTestDOM(`
        <div id="banner" data-ad-format="horizontal">Horizontal Ad</div>
        <div id="sidebar" data-ad-format="vertical">Vertical Ad</div>
        <div id="popup" data-sponsored="true">
          <span>Sponsored</span>
          <span>Content</span>
        </div>
        <div id="gtm" gtm-module="tracking">GTM module</div>
        <div id="content" class="article">Normal content</div>
        <div id="recommendation" data-recommendation="products">Product recommendations</div>
      `);

      const rules = `
        div:matches-attr(*ad*)
        div:matches-attr(data-sponsored=true) > *
        div:matches-attr(/^gtm-/)
      `;

      startEngine(rules);

      expect(getVisibleElements('#banner')).toHaveLength(0);
      expect(getVisibleElements('#sidebar')).toHaveLength(0);
      expect(getVisibleElements('#popup')).toHaveLength(1);
      expect(getVisibleElements('#popup > *')).toHaveLength(0);
      expect(getVisibleElements('#gtm')).toHaveLength(0);
      expect(getVisibleElements('#content')).toHaveLength(1);
      expect(getVisibleElements('#recommendation')).toHaveLength(1);
    });
  });

  describe(':style() rules', () => {
    test('applies style without hiding element', () => {
      createTestDOM(`
        <div class="styled">Styled content</div>
        <div id="nonstyled">Non styled content</div>
      `);

      startEngine('.styled:style(color: rgb(255, 0, 0))');

      const el = document.querySelector('.styled')!;
      expect(getVisibleElements('.styled')).toHaveLength(1);
      expect(getComputedStyle(el).color).toBe('rgb(255, 0, 0)');

      const nonStyledEl = document.querySelector('#nonstyled')!;
      expect(getComputedStyle(nonStyledEl).color).not.toBe('rgb(255, 0, 0)');
    });

    test('applies multiple declarations', () => {
      createTestDOM(`
        <div class="box">Box</div>
      `);

      startEngine('.box:style(color: rgb(0, 128, 0);background-color: rgb(0, 0, 0))');

      const el = document.querySelector('.box')!;
      const style = getComputedStyle(el);
      expect(style.color).toBe('rgb(0, 128, 0)');
      expect(style.backgroundColor).toBe('rgb(0, 0, 0)');
    });

    test('applies multiple rules', () => {
      createTestDOM(`
        <div class="box">Box</div>
      `);

      startEngine(`
        .box:style(visibility: hidden)
        .box:style(color: blue)
      `);

      const el = document.querySelector('.box')!;
      const style = getComputedStyle(el);
      expect(style.visibility).toBe('hidden');
      expect(style.color).toBe('blue');
    });

    test('updates styles when rule match swaps', async () => {
      jest.useFakeTimers();
      createTestDOM(`
        <div class="swap first">Swap</div>
      `);

      startEngine(`
        .first:style(color: rgb(255, 0, 0))
        .second:style(color: rgb(0, 0, 255))
      `);

      const el = document.querySelector('.swap')!;
      expect(getComputedStyle(el).color).toBe('rgb(255, 0, 0)');

      el.classList.remove('first');
      el.classList.add('second');

      await jest.runAllTimersAsync();

      expect(getComputedStyle(el).color).toBe('rgb(0, 0, 255)');
      jest.useRealTimers();
    });

    test('ignores invalid :style() rule but applies other rules', () => {
      createTestDOM(`
        <div class="ok">Ok</div>
        <div class="bad">Bad</div>
      `);

      const rules = `
        .ok:style(color: rgb(0, 0, 0))
        .bad:style(color: )
      `;

      expect(() => startEngine(rules)).not.toThrow();
      const ok = document.querySelector('.ok')!;
      expect(getComputedStyle(ok).color).toBe('rgb(0, 0, 0)');
      expect(getVisibleElements('.bad')).toHaveLength(1);
    });

    test('applies styles to dynamically added elements', async () => {
      jest.useFakeTimers();
      createTestDOM('<div class="container"></div>');

      startEngine('.dynamic:style(color: rgb(128, 0, 128))');

      const el = document.createElement('div');
      el.className = 'dynamic';
      el.textContent = 'Dynamic';
      document.body.appendChild(el);

      await jest.runAllTimersAsync();

      expect(getComputedStyle(el).color).toBe('rgb(128, 0, 128)');
      jest.useRealTimers();
    });
  });
});
