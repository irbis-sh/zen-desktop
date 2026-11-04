/** @type {import('ts-jest').JestConfigWithTsJest} **/
module.exports = {
  testEnvironment: './src/test/environment.ts',
  globalSetup: './src/test/global-setup.ts',
  transform: {
    '^.+.tsx?$': ['ts-jest', {}],
  },
};
