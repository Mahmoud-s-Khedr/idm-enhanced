module.exports = {
  forbidden: [
    { name: 'no-cycles', severity: 'error', from: {}, to: { circular: true } },
    {
      name: 'frontend-no-extension',
      severity: 'error',
      from: { path: '^frontend/' },
      to: { path: '^extensions/' },
    },
    {
      name: 'extension-no-frontend',
      severity: 'error',
      from: { path: '^extensions/' },
      to: { path: '^frontend/' },
    },
    {
      name: 'shared-no-browser-adapter',
      severity: 'error',
      from: { path: '^extensions/shared/' },
      to: { path: '^extensions/(firefox|chromium)/' },
    },
  ],
  options: {
    doNotFollow: { path: 'node_modules|frontend/bindings' },
    tsConfig: { fileName: 'tsconfig.json' },
    enhancedResolveOptions: { extensions: ['.ts', '.tsx', '.js', '.json'] },
  },
};
