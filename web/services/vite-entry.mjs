import {servicesBuildMode} from '../src/services/mode.ts';

/** The existing root index and browser guard remain authoritative. */
export function selectServicesEntry(html, env) {
  if (!servicesBuildMode(env)) return html;
  const entries = [...html.matchAll(/<script\b[^>]*>/g)].filter(([tag]) => /\bsrc=["']\/src\/main\.tsx["']/.test(tag));
  if (entries.length !== 1 || !/\btype=["']module["']/.test(entries[0][0]))
    throw new Error('Services requires exactly one root module entry.');
  return html.replace(entries[0][0], entries[0][0].replace('/src/main.tsx', '/src/services/main.tsx'));
}

/** @returns {import('vite').Plugin} */
export function servicesEntryPlugin(env) {
  const selected = {VITE_HOSTED_WEB: env.VITE_HOSTED_WEB, VITE_ACCOUNT_WORKSPACE: env.VITE_ACCOUNT_WORKSPACE};
  return {name: 'portico-services-entry', transformIndexHtml: {order: 'pre', handler: (html, context) =>
    context.path === '/index.html' || context.path === '/' ? selectServicesEntry(html, selected) : html}};
}
