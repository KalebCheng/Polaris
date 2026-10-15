export default {
  plugins: ['typescript', 'react', 'unicorn'],
  categories: {
    correctness: 'error',
    suspicious: 'warn',
  },
  env: {
    browser: true,
    es2024: true,
  },
  ignorePatterns: ['dist', 'node_modules', 'coverage'],
  rules: {
    'no-console': 'warn',
  },
}
