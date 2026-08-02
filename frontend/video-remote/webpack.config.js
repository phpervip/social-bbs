const { ModuleFederationPlugin } = require('webpack').container;

/**
 * Video Remote — consumed by the Shell host (frontend/shell, :3000) at runtime.
 * Pure remote: exposes the upload / player / list components, consumes @b/shared
 * from the host's share scope (import: false — never bundles its own copy).
 */
module.exports = {
  name: 'video',
  // Standard MF remote: plain entry — the ModuleFederationPlugin rewrites the
  // entry chunk and emits it as remoteEntry.js WITH the container object
  // (window.video = {get, init}) inside. Explicitly naming the entry
  // ({ remoteEntry: './src/index.js' }) would instead emit a bare runtime
  // remoteEntry.js and push the container into a separate video.js chunk the
  // host never loads — "window.video is undefined".
  entry: './src/index.js',
  output: {
    filename: '[name].js',
    publicPath: 'auto',
    clean: true,
  },
  resolve: {
    extensions: ['.jsx', '.js'],
  },
  devServer: {
    port: 3003,
    historyApiFallback: true,
    hot: true,
    headers: {
      // MF cross-origin REQUIRED — Shell fetches remoteEntry.js from :3003
      'Access-Control-Allow-Origin': '*',
    },
  },
  module: {
    rules: [
      {
        test: /\.jsx?$/,
        exclude: /node_modules/,
        use: {
          loader: 'babel-loader',
          options: {
            presets: [
              ['@babel/preset-env', { targets: { browsers: ['last 2 versions', 'not dead'] } }],
              ['@babel/preset-react', { runtime: 'automatic' }],
            ],
          },
        },
      },
      {
        test: /\.css$/,
        use: ['style-loader', 'css-loader'],
      },
    ],
  },
  plugins: [
    new ModuleFederationPlugin({
      name: 'video',
      // MUST be explicit: the Shell host fetches `video@http://localhost:3003/remoteEntry.js`
      // (shell webpack.config.js remotes). Without this, the container chunk is
      // emitted as `video.js` and remoteEntry.js 404s -> "window.video is undefined".
      filename: 'remoteEntry.js',
      exposes: {
        './VideoUpload': './src/components/VideoUpload.jsx',
        './VideoPlayer': './src/components/VideoPlayer.jsx',
        './VideoList': './src/components/VideoList.jsx',
      },
      shared: {
        react: { singleton: true, requiredVersion: false },
        'react-dom': { singleton: true, requiredVersion: false },
        'react-router-dom': { singleton: true, requiredVersion: false },
        axios: { singleton: true },
        // Consume the host instance — DO NOT provide our own.
        '@b/shared': { singleton: true, import: false, requiredVersion: false },
      },
    }),
  ],
};