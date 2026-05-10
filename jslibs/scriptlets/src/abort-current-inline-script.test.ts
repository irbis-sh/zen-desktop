import { expect, test, describe } from '@jest/globals';

import { abortCurrentInlineScript } from './abort-current-inline-script';

describe('abort-current-inline-script', () => {
  afterEach(() => {
    delete (window as any).PROPERTY;
    delete (window as any).test;
    delete (window as any).prop1;
  });

  test('single prop getter', () => {
    abortCurrentInlineScript('test');

    setNewScript();

    expect(() => {
      (window as any).test;
    }).toThrow(ReferenceError);
  });

  test('single prop setter', () => {
    abortCurrentInlineScript('test');

    setNewScript();

    expect(() => {
      (window as any).test = 123;
    }).toThrow(ReferenceError);
  });

  test('property on existing object', () => {
    (window as any).prop1 = {};

    abortCurrentInlineScript('prop1.prop2');

    setNewScript();

    expect(() => {
      (window as any).prop1.prop2;
    }).toThrow(ReferenceError);
  });

  test('test.prop.prop2 getter', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortCurrentInlineScript('test.prop.prop2');

    setNewScript();

    expect(() => {
      (window as any).test.prop.prop2;
    }).toThrow(ReferenceError);
  });

  test('document.querySelectorAll', () => {
    abortCurrentInlineScript('document.querySelectorAll');

    setNewScript();

    expect(() => {
      window.document.querySelectorAll('test');
    }).toThrow(ReferenceError);
  });

  test('properties inside chain are not initialized by scriptlet', () => {
    abortCurrentInlineScript('prop1.prop2.prop3');

    expect((window as any).prop1).toBeUndefined();

    (window as any).prop1 = {};

    expect((window as any).prop1.prop2).toBeUndefined();

    (window as any).prop1.prop2 = {};

    expect((window as any).prop1.prop2.prop3).toBeUndefined();

    (window as any).prop1.prop2.prop3 = 123;

    setNewScript();
    expect(() => {
      (window as any).prop1.prop2.prop3;
    }).toThrow(ReferenceError);
  });

  test('matching substring throws an error', () => {
    abortCurrentInlineScript('prop1', 'test');
    setNewScript('test');

    expect(() => {
      (window as any).prop1;
    }).toThrow(ReferenceError);
  });

  test('non-matching substring does not throw an error', () => {
    abortCurrentInlineScript('prop1', 'test');
    setNewScript('xdd');

    expect(() => {
      (window as any).prop1;
    }).not.toThrow(ReferenceError);
  });

  test('matching regex throws an error', () => {
    abortCurrentInlineScript('prop1', '/(?<!^)test(?!$)/');
    setNewScript('some__test__some');

    expect(() => {
      (window as any).prop1;
    }).toThrow(ReferenceError);
  });

  test('non-matching regex does not throw an error', () => {
    abortCurrentInlineScript('prop1', '/(?<!^)test(?!$)/');
    setNewScript('test');

    expect(() => {
      (window as any).prop1;
    }).not.toThrow(ReferenceError);
  });

  test('getter retains this', () => {
    const test = {
      data: 123,
    };
    Object.defineProperty(test, 'dataGetter', {
      configurable: true,
      get() {
        return this.data;
      },
    });

    (window as any).test = test;
    abortCurrentInlineScript('test.dataGetter', '🥸');

    expect((window as any).test.dataGetter).toEqual(123);
  });

  test('setter retains this', () => {
    const test = {
      data: 0,
    };
    Object.defineProperty(test, 'dataSetter', {
      configurable: true,
      set(v) {
        this.data = v;
      },
    });

    (window as any).test = test;
    abortCurrentInlineScript('test.dataSetter', '🥸');

    (window as any).test.dataSetter = 123;

    expect((window as any).test.data).toEqual(123);
  });
});

function setNewScript(textContent?: string) {
  const newScript = document.createElement('script');
  if (textContent) {
    newScript.textContent = textContent;
  }
  Object.defineProperty(document, 'currentScript', {
    configurable: true,
    get: () => newScript,
  });
}
