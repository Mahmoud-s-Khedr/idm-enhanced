import tseslint from 'typescript-eslint';
export default tseslint.config(
  {
    ignores: [
      '**/dist/**',
      'frontend/bindings/**',
      '.tools/**',
      'coverage/**',
      'build/**',
      // MPEG-TS binary fixture data shares the .ts suffix.
      'testdata/hls/**/*.ts',
    ],
  },
  ...tseslint.configs.recommended,
  {
    files: ['**/*.ts', '**/*.tsx'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/consistent-type-imports': 'error',
    },
  },
);
