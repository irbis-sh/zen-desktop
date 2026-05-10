module.exports = {
  root: true,
  env: {
    browser: true,
  },
  parser: '@typescript-eslint/parser',
  plugins: ['@typescript-eslint', 'prettier'],
  extends: [
    'eslint:recommended',
    'plugin:import/recommended',
    'plugin:import/typescript',
    'plugin:prettier/recommended',
  ],
  rules: {
    'import/order': [
      'error',
      {
        'newlines-between': 'always',
        groups: ['builtin', 'external', 'internal', 'parent', 'sibling', 'index'],
        alphabetize: {
          order: 'asc',
          caseInsensitive: true,
        },
      },
    ],
    'import/prefer-default-export': 0,
    '@typescript-eslint/no-use-before-define': 0,
    '@typescript-eslint/no-shadow': 0,
    'import/no-relative-packages': 0,
    'no-undef': 0,
    'no-console': 1,
    'no-unused-vars': 'off',
    '@typescript-eslint/no-unused-vars': ['error'],
  },
  parserOptions: {
    project: './tsconfig.json',
    tsconfigRootDir: __dirname,
  },
  ignorePatterns: ['node_modules/'],
};
