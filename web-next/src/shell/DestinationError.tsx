import {Component, type ErrorInfo, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {StateLayout} from '../kit/states';
import {router} from '../router';

/**
 * The destination recovery witness: the actual Router location object, its href, the actual history
 * entry key when the router supplies one, and the captured owner scope. Identity is the recovery
 * lifetime — a new location object (including a same-href fresh entry), a real entry change or a
 * scope replacement creates a new witness; a missing entry key never collapses the href.
 */
export type DestinationRecoveryWitness = Readonly<{
  location: unknown;
  href: string;
  entry?: string;
  scopeKey?: string;
}>;

export type DestinationErrorProps = Readonly<{
  witness: DestinationRecoveryWitness;
  children: ReactNode;
  /** Test/owner port: defaults to the existing router's Home navigation. */
  onHome?: () => void;
}>;

type DestinationErrorState = Readonly<{failed: boolean}>;

/**
 * The one destination error presenter: a failed destination keeps the shell mounted and offers a
 * bounded Try again (this boundary resets only its own destination) and Home (the existing router).
 * It wraps the outlet only, so a ShellRoot failure stays an app-level failure for the last resort.
 */
export class DestinationErrorBoundary extends Component<DestinationErrorProps, DestinationErrorState> {
  state: DestinationErrorState = {failed: false};
  static getDerivedStateFromError(): DestinationErrorState { return {failed: true}; }
  componentDidCatch(_error: Error, _info: ErrorInfo) { /* The presenter is the report; nothing else is replaced. */ }
  componentDidUpdate(previous: DestinationErrorProps) {
    // A new witness is a new recovery lifetime: a same-href fresh entry, an actual entry change or
    // a scope replacement must not leave the failed destination latched.
    if (previous.witness !== this.props.witness && this.state.failed) this.setState({failed: false});
  }
  private reset = () => this.setState({failed: false});
  private home = () => {
    if (this.props.onHome) { this.props.onHome(); return; }
    void router.navigate({to: '/'}).catch(() => { /* The existing shell reports navigation failure. */ });
  };
  render() {
    if (!this.state.failed) return this.props.children;
    const t = defaultI18n.t;
    return <StateLayout title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} icon="i-warn" tone="warning"
      primary={{id: 'retry', label: t('next.states.tryAgain'), icon: 'i-retry', onPress: this.reset}}
      secondary={{id: 'home', label: t('next.states.goHome'), onPress: this.home}} />;
  }
}
