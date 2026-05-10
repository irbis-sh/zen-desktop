import resolve from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
import typescript from '@rollup/plugin-typescript';
import babel from '@rollup/plugin-babel';
import terser from '@rollup/plugin-terser';

export default {
  input: 'src/index.ts',
  output: {
    file: 'dist/bundle.js',
    format: 'iife',
    name: 'extendedCSS',
  },
  plugins: [
    resolve({
      // https://github.com/csstree/csstree/issues/278#issuecomment-2336642879
      browser: true,
      preferBuiltins: false,
    }),
    commonjs(),
    typescript({
      exclude: ['**/test/*.ts', '**/*.test.ts'],
    }),
    babel({
      babelHelpers: 'bundled',
      presets: [
        [
          '@babel/preset-env',
          {
            targets: '> 0.2%, not dead',
            useBuiltIns: 'usage',
            corejs: '3.41',
          },
        ],
      ],
      exclude: 'node_modules/**',
      extensions: ['.js', '.ts'],
    }),
    terser(),
  ],
};
