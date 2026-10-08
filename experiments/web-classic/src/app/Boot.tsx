import s from './Boot.module.css';

/** Start-up screen while the service layer and the saved session load. */
export function Splash() {
  return (
    <div className={s.splash} role="status" aria-label="Starting Portico">
      <img src="/brand/portico-wordmark-dark.svg" alt="Portico" />
      <div className={s.bar} />
    </div>
  );
}
