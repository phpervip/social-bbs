const path = require('path');
const { ModuleFederationPlugin } = require('webpack').container;

/**
 * Feed Remote — consumed by the Shell host (frontend/shell, :3000) at runtime.
 * Pure remote: exposes the three page components, consumes @b/shared from the
 * host's share scope (import: false — never bundles its own copy).
 */
module.exports = {
  name: 'feed',
  // The entry chunk carries the Module Federation container and MUST be
  // emitted as remoteEntry.js — that's the URL the Shell host fetches.
  entry: { remoteEntry: './src/index.js' },
  output: {
    publicPath: 'auto',
    clean: true,
  },
  resolve: {
    extensions: ['.jsx', '.js'],
  },
  devServer: {
    port: 3001,
    historyApiFallback: true,
    hot: true,
    headers: {
      // MF cross-origin REQUIRED — Shell fetches remoteEntry.js from :3001
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
      name: 'feed',
      exposes: {
        './HomeTimeline': './src/HomeTimeline.jsx',
        './PostDetail': './src/PostDetail.jsx',
        './Explore': './src/Explore.jsx',
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
