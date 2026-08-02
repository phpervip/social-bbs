const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');

/**
 * Standalone dev build (npm run dev:standalone) — lets you develop the User
 * UI in isolation WITHOUT the Shell host. Aliases '@b/shared' to a local
 * shim (dev/shared-shim.js). NOT part of the Module Federation build.
 */
module.exports = {
  name: 'user-standalone',
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
    port: 3002,
    historyApiFallback: true,
    hot: true,
    // Array form REQUIRED — webpack-dev-server 5.2.x validates `proxy` as an
    // array of { context, target } (feed-remote's object map fails the same
    // schema check on 5.2.6; this keeps dev:standalone runnable).
    proxy: [
      {
        // Gateway REST API (public auth routes + token'd user routes)
        context: ['/api'],
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    ],
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
