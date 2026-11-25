const path = require('path');

module.exports = {
    entry: './src/index.js',
    output: {
        path: path.resolve(__dirname, 'dist'),
        filename: 'main.js',
        clean: true,
    },
    // React Mattermost отдаёт плагинам глобально: в бандл его не тянем.
    externals: {
        react: 'React',
        'react-dom': 'ReactDOM',
    },
    module: {
        rules: [
            {
                test: /\.(js|jsx)$/,
                exclude: /node_modules/,
                use: {
                    loader: 'babel-loader',
                    options: {
                        // runtime classic: без импорта react/jsx-runtime, который
                        // Mattermost 12 выносит наружу отдельно.
                        presets: [
                            ['@babel/preset-env', {targets: {chrome: '100'}}],
                            ['@babel/preset-react', {runtime: 'classic'}],
                        ],
                    },
                },
            },
        ],
    },
    resolve: {
        extensions: ['.js', '.jsx'],
    },
    devtool: false,
};
