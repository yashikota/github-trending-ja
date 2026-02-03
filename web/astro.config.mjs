// @ts-check
import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';

// https://astro.build/config
export default defineConfig({
  output: 'static',

  site: process.env.SITE_URL || 'https://github-trending-ja.yashikota.com',

  redirects: {
    '/feed': 'https://yashikota.github.io/github-trending-ja/feed.xml',
  },

  build: {
    inlineStylesheets: 'auto',
  },

  vite: {
    plugins: [tailwindcss()]
  }
});
