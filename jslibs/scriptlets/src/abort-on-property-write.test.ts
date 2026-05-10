import { expect, test, describe } from '@jest/globals';

import { abortOnPropertyWrite } from './abort-on-property-write';

describe('abort-on-property-write', () => {
  afterEach(() => {
    delete (window as any).PROPERTY;
    delete (window as any).test;
    delete (window as any).prop1;
  });

  test('abort on single prop write', () => {
    abortOnPropertyWrite('test');

    expect(() => {
      (window as any).test = '123';
    }).toThrow(ReferenceError);
  });

  test('dont abort on prop read', () => {
    abortOnPropertyWrite('test');

    expect(() => {
      (window as any).test;
    }).not.toThrow(ReferenceError);
  });

  test('abort on long chain write', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortOnPropertyWrite('test.prop.prop2');

    expect(() => {
      (window as any).test.prop.prop2 = 123;
    }).toThrow(ReferenceError);
  });

  test('dont abort on long chain read', () => {
    (window as any).test = {
      prop: {
        prop2: () => {},
      },
    };

    abortOnPropertyWrite('test.prop.prop2');

    expect(() => {
      (window as any).test.prop.prop2;
    }).not.toThrow(ReferenceError);
  });

  test('document.querySelectorAll write', () => {
    abortOnPropertyWrite('document.querySelectorAll');

    expect(() => {
      (window.document.querySelectorAll as any) = () => {};
    }).toThrow(ReferenceError);
  });

  test('properties inside chain are not initialized by scriptlet', () => {
    abortOnPropertyWrite('prop1.prop2.prop3');

    expect((window as any).prop1).toBeUndefined();

    (window as any).prop1 = {};

    expect((window as any).prop1.prop2).toBeUndefined();

    (window as any).prop1.prop2 = {};

    expect((window as any).prop1.prop2.prop3).toBeUndefined();

    expect(() => {
      (window as any).prop1.prop2.prop3 = '123';
    }).toThrow(ReferenceError);
  });
});
