import {Component, type ErrorInfo, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {StateLayout} from '../kit/states';
import {router} from '../router';

export type DestinationErrorProps = Readonly<{
  /**
   * The recovery lifetime of the destination entry: the actual route key (href plus the router's
   * history entry) combined with the captured owner scope. A fresh entry with the same href, or an
   * authority/scope replacement, changes this value and unlatches a failed destination.
   */
  lifetime: string;
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
    // A fresh entry (same href, new history entry) or a scope replacement is a new recovery
    // lifetime: the failed destination must not stay latched across it.
    if (previous.lifetime !== this.props.lifetime && this.state.failed) this.setState({failed: false});
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
