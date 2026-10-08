import {cx} from './cx';

/** Approved brand assets only; never rebuilt from text. */
export function Wordmark({height = 22, className}: {height?: number; className?: string}) {
  return <img src="/assets/portico-wordmark-mono-white.svg" alt="Portico" height={height} className={cx(className)} draggable={false} />;
}
export function BrandMark({size = 28, className}: {size?: number; className?: string}) {
  return <img src="/assets/portico-symbol-mono-white.svg" alt="" aria-hidden width={size} height={size} className={cx(className)} draggable={false} />;
}
