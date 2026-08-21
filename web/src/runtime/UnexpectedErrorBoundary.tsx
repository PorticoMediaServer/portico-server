import { productMessage } from '@portico/client-core';
import { Component, type ErrorInfo, type ReactNode, createRef } from 'react';
import { ProductProblemMessage } from './ProductProblemMessage';

type UnexpectedErrorBoundaryProps = { children: ReactNode };
type UnexpectedErrorBoundaryState = { failed: boolean };

export function logUnexpectedRenderFailure(error: Error, info: ErrorInfo, development = import.meta.env.DEV) {
  if (development) console.error('Portico encountered an unexpected render failure.', { error, componentStack: info.componentStack });
  else console.error('Portico encountered an unexpected render failure.');
}

export class UnexpectedErrorBoundary extends Component<UnexpectedErrorBoundaryProps, UnexpectedErrorBoundaryState> {
  state: UnexpectedErrorBoundaryState = { failed: false };
  private heading = createRef<HTMLHeadingElement>();

  static getDerivedStateFromError(): UnexpectedErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    logUnexpectedRenderFailure(error, info);
    this.heading.current?.focus();
  }

  componentDidUpdate(_: UnexpectedErrorBoundaryProps, previous: UnexpectedErrorBoundaryState) {
    if (!previous.failed && this.state.failed) this.heading.current?.focus();
  }

  render() {
    if (!this.state.failed) return this.props.children;

    return <main className="auth-surface runtime-surface">
      <section className="auth-panel runtime-panel">
        <img src="/brand/portico-wordmark-white.svg" alt="Portico" />
        <h1 className="runtime-unexpected-focus" ref={this.heading} tabIndex={-1}>{productMessage('problem.request-failed').title}</h1>
        <ProductProblemMessage className="runtime-recovery-problem" fallbackId="problem.request-failed" additionalActions={['action.reload']} actionHandlers={{
          'action.retry': () => this.setState({ failed: false }),
          'action.reload': () => window.location.reload(),
        }} showTitle={false} />
      </section>
    </main>;
  }
}
