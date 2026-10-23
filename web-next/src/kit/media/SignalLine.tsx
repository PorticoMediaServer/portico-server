import {forwardRef, type ButtonHTMLAttributes} from 'react';
import {Icon} from '../Icon';
import {WaveGlyph} from './WaveGlyph';
import type {MediaLayout, SignalPathModel, SignalTranslate} from './types';
import styles from './media.module.css';

export type SignalLineProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children' | 'translate'> & Readonly<{model: SignalPathModel; translate: SignalTranslate; layout?: MediaLayout}>;
export const SignalLine = forwardRef<HTMLButtonElement, SignalLineProps>(function SignalLine({model, translate, layout = 'desktop', className, ...props}, ref) {
  const source = layout === 'desktop' ? model.source : model.shortSource ?? model.source;
  const state = model.state ? translate(model.state) : undefined;
  const at = state && model.stateEmphasis ? state.indexOf(model.stateEmphasis) : -1;
  return <button {...props} type="button" ref={ref} className={[styles.signal, className].filter(Boolean).join(' ')} data-layout={layout} data-healthy={model.greenGlyph || undefined} aria-label={translate(model.a11y)}>
    <WaveGlyph size={layout === 'tv' ? 26 : layout === 'phone' ? 14 : 16} />
    {source && <b>{translate(source)}</b>}
    {source && state && <i className={styles.signalGap} />}
    {state && <span className={styles.signalState}>{at < 0 || !model.stateEmphasis ? state : <>{state.slice(0, at)}<b>{model.stateEmphasis}</b>{state.slice(at + model.stateEmphasis.length)}</>}</span>}
    {layout !== 'phone' && <Icon name="i-right" size={layout === 'tv' ? 20 : 14} className={styles.chevron} />}
  </button>;
});
