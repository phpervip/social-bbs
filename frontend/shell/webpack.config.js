const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const { ModuleFederationPlugin } = require('webpack').container;

/**
 * B — Shell micro-frontend host.
 * Port 3000. Consumes Feed Remote (:3001) via Module Federation.
 * Shares the app-level `@b/shared` module (api client + event bus + UI kit).
 */
module.exports = (env, argv) => {
  const isProd = argv.mode === 'production';
  return {
    name: 'shell',
    entry: path.resolve(__dirname, 'src/index.js'),
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: isProd ? 'static/js/[name].[contenthash:8].js' : 'static/js/[name].js',
      publicPath: 'auto',
      clean: true,
    },
    resolve: {
      extensions: ['.js', '.jsx'],
    },
    module: {
      rules: [
        {
          test: /\.(js|jsx)$/,
          exclude: /node_modules/,
          use: {
            loader: 'babel-loader',
            options: {
              presets: [
                ['@babel/preset-env', { targets: 'defaults' }],
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
        name: 'shell',
        remotes: {
          feed: 'feed@http://localhost:3001/remoteEntry.js',
          user: 'user@http://localhost:3002/remoteEntry.js',
        },
        shared: {
          // Host-provided deps must be eager so async-consumed remotes (and the
          // host's own async bootstrap chunk) never hit "Shared module is not
          // available for eager consumption".
          react: { singleton: true, requiredVersion: false, eager: true },
          'react-dom': { singleton: true, requiredVersion: false, eager: true },
          'react-router-dom': { singleton: true, requiredVersion: false, eager: true },
          axios: { singleton: true, eager: true },
          // App-level shared module — THE CONTRACT. Remotes import { api, bus, ui } from this.
          '@b/shared': { import: './src/shared', singleton: true, eager: true },
        },
      }),
      new HtmlWebpackPlugin({
        template: path.resolve(__dirname, 'src/index.html'),
        title: 'B',
      }),
    ],
    devServer: {
      port: 3000,
      historyApiFallback: true,
      hot: true,
      open: false,
      proxy: [
        {
          context: ['/api'],
          target: 'http://localhost:8080',
        },
      ],
      static: {
        directory: path.resolve(__dirname, 'dist'),
      },
    },
    performance: { hints: false },
  };
};
