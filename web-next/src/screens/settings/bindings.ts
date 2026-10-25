import type {ReactNode} from 'react';
import type {SettingsRow, SettingsValue} from '@core/presentation/settings-spec.ts';
import type {SettingPresentation} from '../../kit/settings';
import type {ShellIdentity} from '../../shell/Shell';
import type {SettingsBindings, SettingsContext} from './Settings';
import type {SettingsController, SettingsControllerOptions} from './controller';
import type {SignInSettingsSource} from './signin-source';

export type SettingsSections = Readonly<{
  openHome: () => void;
  special: (row: SettingsRow, state: SettingPresentation, context: SettingsContext, openPanel: (panelId: string, recordId?: string) => void) => ReactNode;
  section?: SettingsBindings['section'];
  panel?: SettingsBindings['panel'];
  change?: (row: SettingsRow, value: SettingsValue, context: SettingsContext) => boolean;
  reset?: (row: SettingsRow, context: SettingsContext) => boolean;
  signIn?: SignInSettingsSource;
}>;

/** The parent supplies actual domain adapters. This layer owns no query resolver or key map. */
export function createSettingsBindings(controller: SettingsController, options: SettingsControllerOptions, identity: ShellIdentity, sections: SettingsSections): SettingsBindings {
  /** A switch row belongs to the direction its server is not already in. Both
   * drawn directions have their own surface; without the captured owner switch
   * they stay out rather than becoming dead controls. The local change-owner
   * row stays out because there is no justified local-only transfer authority. */
  const signInSwitch = (rowId: string): {present: boolean; disabled?: boolean} | undefined => {
    if (rowId === 'signin.changeOwnerLocal') return {present: false};
    if (rowId === 'signin.changeOwner') {
      if (!sections.signIn?.ownerSwitch) return {present: false};
      const snapshot = sections.signIn.getSnapshot();
      return {present: !!snapshot.document && snapshot.document.signIn === 'portico', disabled: snapshot.loading};
    }
    // The Change group offers the opposite direction from the one in use.
    const want = rowId === 'signin.switchToPortico' || rowId === 'signin.startOverPortico' ? 'local'
      : rowId === 'signin.switchToLocal' || rowId === 'signin.startOverLocal' ? 'portico' : undefined;
    if (!want) return undefined;
    if (!sections.signIn?.ownerSwitch) return {present: false};
    const snapshot = sections.signIn.getSnapshot();
    return {present: !!snapshot.document && snapshot.document.signIn === want, disabled: snapshot.loading};
  };
  return {
    accountName: identity.profileName,
    serverName: identity.serverName,
    role: options.role,
    environment: {...options.environment, role: options.role, signIn: identity.signIn},
    subscribe: listener => {
      const offController = controller.subscribe(listener), offSignIn = sections.signIn?.subscribe(listener);
      return () => {offController();offSignIn?.();};
    },
    getSnapshot: section => section.scope === 'server' && section.id === 'signin' && sections.signIn ? sections.signIn.getSnapshot() : controller.getSnapshot(section),
    sectionVisible: controller.sectionVisible,
    eligible: (row, context) => {
      const fact = signInSwitch(row.id) ?? sections.signIn?.rowState(row.id);
      return fact ? controller.sectionVisible(context.section) && fact.present : controller.eligible(row, context);
    },
    audience: section => ['dashboard', 'libraries', 'people'].includes(section.id) ? 'administrators' : 'owner',
    serverHealth: controller.serverHealth,
    serverFacts: controller.serverFacts,
    rowState: (row, context) => signInSwitch(row.id) ?? sections.signIn?.rowState(row.id) ?? controller.rowState(row, context),
    change: (row, value, context) => {if (!sections.change?.(row, value, context)) controller.change(row, value, context);},
    reset: (row, context) => {if (!sections.reset?.(row, context)) controller.reset(row, context);},
    configurationMatches: controller.configurationMatches,
    special: sections.special,
    section: sections.section,
    panel: sections.panel,
    load: section => section.scope === 'server' && section.id === 'signin' && sections.signIn && controller.sectionVisible(section) ? sections.signIn.load() : controller.load(section),
    loadContext: controller.loadContext,
    openHome: sections.openHome,
  };
}
