const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const { ModuleFederationPlugin } = require('webpack').container;

/**
 * B — Shell micro-frontend host.
 * Port 3000. Consumes Feed Remote (:3001), User Remote (:3002) and
 * Video Remote (:3003) via dynamic (lazy) Module Federation.
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
        // Dynamic (lazy) remotes: fetch remoteEntry.js at runtime via <script>
        // injection so the shell compiles & boots even when a remote is down.
        // Unreachable remotes resolve to a stub whose get() rejects — React.lazy
        // throws and ErrorBoundary renders the fallback instead of crashing.
        remotes: {
          feed: `promise new Promise((resolve) => {
            const name = 'feed', url = 'http://localhost:3001/remoteEntry.js';
            const script = document.createElement('script');
            script.src = url;
            script.onload = () => resolve({
              get: (req) => window[name].get(req),
              init: (arg) => { try { return window[name].init(arg); } catch (e) { console.warn('[mf] feed init', e); } },
            });
            script.onerror = () => resolve({
              get: (req) => Promise.reject(new Error('feed remote unavailable while loading ' + req)),
              init: () => undefined,
            });
            document.head.appendChild(script);
          })`,
          user: `promise new Promise((resolve) => {
            const url = 'http://localhost:3002/remoteEntry.js';
            const script = document.createElement('script');
            script.src = url;
            script.onload = () => resolve({
              get: (req) => window.user.get(req),
              init: (arg) => { try { return window.user.init(arg); } catch (e) { console.warn('[user] init', e); } },
            });
            script.onerror = () => resolve({
              get: (req) => Promise.reject(new Error('user remote unavailable while loading ' + req)),
              init: () => undefined,
            });
            document.head.appendChild(script);
          })`,
          video: `promise new Promise((resolve) => {
            const url = 'http://localhost:3003/remoteEntry.js';
            const script = document.createElement('script');
            script.src = url;
            script.onload = () => resolve({
              get: (req) => window.video.get(req),
              init: (arg) => { try { return window.video.init(arg); } catch (e) { console.warn('[video] init', e); } },
            });
            script.onerror = () => resolve({
              get: (req) => Promise.reject(new Error('video remote unavailable while loading ' + req)),
              init: () => undefined,
            });
            document.head.appendChild(script);
          })`,
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
