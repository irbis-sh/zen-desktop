import { expect, test, describe } from '@jest/globals';

import { abortOnStackTrace } from './abort-on-stack-trace';

describe('abort-on-stack-trace', () => {
  afterEach(() => {
    delete (window as any).PROPERTY;
    delete (window as any).test;
    delete (window as any).prop1;
  });
  test('abort on matching stack', () => {
    abortOnStackTrace('test', 'jest');

    expect(() => {
      (window as any).test;
    }).toThrow(ReferenceError);

    expect(() => {
      (window as any).test = '123';
    }).toThrow(ReferenceError);
  });

  test('dont abort on non-matching stack', () => {
    abortOnStackTrace('test', 'ihopethistackwontmatch');

    expect(() => {
      (window as any).test;
    }).not.toThrow(ReferenceError);

    expect(() => {
      (window as any).test = '123';
    }).not.toThrow(ReferenceError);
  });

  test('abort on long chain, matching stack', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortOnStackTrace('test.prop.prop2', 'jest');

    expect(() => {
      (window as any).test.prop.prop2;
    }).toThrow(ReferenceError);

    expect(() => {
      (window as any).test.prop.prop2 = 123;
    }).toThrow(ReferenceError);
  });

  test('dont abort on long chain, non-matching stack', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortOnStackTrace('test.prop.prop2', 'somenonmatchingstack');

    expect(() => {
      (window as any).test.prop.prop2;
    }).not.toThrow(ReferenceError);

    expect(() => {
      (window as any).test.prop.prop2 = '123';
    }).not.toThrow(ReferenceError);
  });

  test('properties inside chain are not initialized by scriptlet', () => {
    abortOnStackTrace('prop1.prop2.prop3', 'someStack');

    expect((window as any).prop1).toBeUndefined();

    (window as any).prop1 = {};

    expect((window as any).prop1.prop2).toBeUndefined();

    (window as any).prop1.prop2 = {};

    expect((window as any).prop1.prop2.prop3).toBeUndefined();
  });
});
