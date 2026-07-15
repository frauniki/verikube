// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import mermaid from 'astro-mermaid';
import starlightLinksValidator from 'starlight-links-validator';

// https://astro.build/config
export default defineConfig({
	site: 'https://frauniki.github.io',
	base: '/verikube',
	integrations: [
		// astro-mermaid must come before starlight so it takes over
		// ```mermaid code fences before Starlight's syntax highlighter.
		mermaid({ theme: 'default', autoTheme: true }),
		starlight({
			title: 'VeriKube',
			description: 'Declarative network checks for Kubernetes.',
			defaultLocale: 'root',
			locales: {
				root: { label: 'English', lang: 'en' },
				ja: { label: '日本語', lang: 'ja' },
			},
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/frauniki/verikube' },
			],
			editLink: {
				baseUrl: 'https://github.com/frauniki/verikube/edit/main/docs/',
			},
			plugins: [
				// The Japanese API reference is intentionally served via
				// root-locale fallback (the page is generated, English-only).
				starlightLinksValidator({ errorOnFallbackPages: false }),
			],
			sidebar: [
				{
					label: 'Getting Started',
					translations: { ja: 'はじめに' },
					items: [
						{ slug: 'getting-started/quickstart' },
						{ slug: 'getting-started/installation' },
					],
				},
				{
					label: 'Concepts',
					translations: { ja: 'コンセプト' },
					items: [{ slug: 'concepts' }],
				},
				{
					label: 'Guides',
					translations: { ja: 'ガイド' },
					items: [
						{ slug: 'guides/writing-checks' },
						{ slug: 'guides/scheduling' },
						{ slug: 'guides/observability' },
						{ slug: 'guides/troubleshooting' },
					],
				},
				{
					label: 'Reference',
					translations: { ja: 'リファレンス' },
					items: [
						{ slug: 'reference/api' },
						{ slug: 'reference/operations' },
					],
				},
				{
					label: 'Security',
					translations: { ja: 'セキュリティ' },
					items: [{ slug: 'security' }],
				},
			],
		}),
	],
});
