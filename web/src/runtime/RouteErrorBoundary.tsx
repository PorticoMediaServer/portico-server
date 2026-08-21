import { productMessage } from '@porticomediaserver/client-core';
import { Home } from '#portico-icons';
import { Component, type ErrorInfo, type ReactNode } from 'react';
import { ProductProblemMessage } from './ProductProblemMessage';
import { recordRouteRenderFailure } from './routeDiagnostics';

type RouteErrorBoundaryProps = {
  children: ReactNode;
  routeKey: string;
};

type RouteErrorBoundaryState = {
  error?: Error;
};

export function logRouteRenderFailure(error: Error, info: ErrorInfo, routeKey = '/', development = import.meta.env.DEV) {
  return recordRouteRenderFailure(error, info, routeKey, development);
}

export class RouteErrorBoundary extends Component<RouteErrorBoundaryProps, RouteErrorBoundaryState> {
  state: RouteErrorBoundaryState = {};

  static getDerivedStateFromError(error: Error): RouteErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    logRouteRenderFailure(error, info, this.props.routeKey);
  }

  componentDidUpdate(previous: RouteErrorBoundaryProps) {
    if (previous.routeKey !== this.props.routeKey && this.state.error) this.setState({ error: undefined });
  }

  render() {
    if (!this.state.error) return this.props.children;
    return <section className="route-failure">
      <ProductProblemMessage className="route-failure-problem" reason={this.state.error} fallbackId="problem.request-failed" actionHandlers={{
        'action.retry': () => this.setState({ error: undefined }),
      }} />
      <div>
        <a className="button secondary" href="/"><Home /> {productMessage('action.go-home').text}</a>
      </div>
    </section>;
  }
}
