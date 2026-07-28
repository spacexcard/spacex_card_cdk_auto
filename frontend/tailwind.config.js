/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        serif: [
          'Source Serif 4', 'Georgia', 'Noto Serif SC', 'Songti SC', 'STSong', 'SimSun', 'serif',
        ],
        sans: [
          'Inter', '-apple-system', 'BlinkMacSystemFont', 'PingFang SC', 'Microsoft YaHei', 'Arial', 'sans-serif',
        ],
      },
      colors: {
        // warm terracotta brand
        terracotta: {
          50: '#f7e9e2',
          100: '#efcfc4',
          200: '#e7bbac',
          300: '#d9947d',
          400: '#ce7558',
          500: '#c0563a',
          600: '#9c4530',
          700: '#7d3626',
        },
        // cream light surfaces
        cream: {
          50: '#fbf8f1',
          100: '#f6f1e7',
          200: '#f2ede4',
          300: '#e7ddcb',
          400: '#dccfb8',
        },
        // warm charcoal dark surfaces
        warm: {
          950: '#17120e',
          900: '#221a14',
          800: '#2b2118',
          700: '#372c21',
          600: '#46382b',
        },
        ink: {
          DEFAULT: '#2a2622',
          muted: '#6f6657',
          subtle: '#9a9183',
        },
      },
    },
  },
  plugins: [],
}
