export function optionalMotionBehavior(): ScrollBehavior {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'auto';
  if (document.documentElement.dataset.porticoReduceMotion === 'true') return 'auto';
  return 'smooth';
}
