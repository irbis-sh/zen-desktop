/** @type {import('ts-jest').JestConfigWithTsJest} **/
module.exports = {
  testEnvironment: "./src/test/environment.ts",
  transform: {
    "^.+.tsx?$": ["ts-jest",{}],
  },
};