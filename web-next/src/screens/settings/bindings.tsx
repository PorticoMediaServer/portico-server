import type {ReactNode} from 'react';
import type {SettingsRow, SettingsValue} from '@core/presentation/settings-spec.ts';
import type {SettingPresentation} from '../../kit/settings';
import type {ShellIdentity} from '../../shell/Shell';
import type {SettingsBindings, SettingsContext} from './Settings';
import type {SettingsController, SettingsControllerOptions} from './controller';

export type SettingsSections = Readonly<{
  openHome: () => void;
  special: (row: SettingsRow, state: SettingPresentation, context: SettingsContext, openPanel: (panelId: string, recordId?: string) => void) => ReactNode;
  section?: SettingsBindings['section'];
  panel?: SettingsBindings['panel'];
  change?: (row: SettingsRow, value: SettingsValue, context: SettingsContext) => boolean;
  reset?: (row: SettingsRow, context: SettingsContext) => boolean;
}>;

/** The parent supplies actual domain adapters. This layer owns no query resolver or key map. */
export function createSettingsBindings(controller: SettingsController, options: SettingsControllerOptions, identity: ShellIdentity, sections: SettingsSections): SettingsBindings {
  return {
    accountName: identity.profileName,
    serverName: identity.serverName,
    role: options.role,
    environment: {...options.environment, role: options.role, signIn: identity.signIn},
    subscribe: controller.subscribe,
    getSnapshot: controller.getSnapshot,
    sectionVisible: controller.sectionVisible,
    eligible: controller.eligible,
    audience: section => ['dashboard', 'libraries', 'people'].includes(section.id) ? 'administrators' : 'owner',
    serverHealth: controller.serverHealth,
    serverFacts: controller.serverFacts,
    rowState: controller.rowState,
    change: (row, value, context) => {if (!sections.change?.(row, value, context)) controller.change(row, value, context);},
    reset: (row, context) => {if (!sections.reset?.(row, context)) controller.reset(row, context);},
    configurationMatches: controller.configurationMatches,
    special: sections.special,
    section: sections.section,
    panel: sections.panel,
    load: controller.load,
    openHome: sections.openHome,
  };
}
