const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');

/**
 * Standalone dev build (npm run dev:standalone) — lets you develop the Feed
 * UI in isolation WITHOUT the Shell host. Aliases '@b/shared' to a local
 * shim (dev/shared-shim.js). NOT part of the Module Federation build.
 */
module.exports = {
  name: 'feed-standalone',
  entry: './dev/entry.js',
  output: {
    path: path.resolve(__dirname, 'dist-standalone'),
    publicPath: 'auto',
    filename: 'bundle.js',
    clean: true,
  },
  resolve: {
    extensions: ['.jsx', '.js'],
    alias: {
      '@b/shared': path.resolve(__dirname, 'dev/shared-shim.js'),
    },
  },
  devServer: {
    port: 3001,
    historyApiFallback: true,
    hot: true,
    proxy: {
      // Gateway dev API (unauthenticated routes /api/dev/* + token'd /api/feed/*)
      '/api': 'http://localhost:8080',
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
    new HtmlWebpackPlugin({
      template: './dev/index.html',
    }),
  ],
};
