import React, {useEffect, useState} from 'react';
import {ScrollView, TVFocusGuideView, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useAdmin} from '../../data/DataProvider';
import type {AdminAction, AdminField, AdminPage} from '../../data/adminTypes';
import {Screen, useChromeInsets, useRegisterNav} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {EmptyState, ListRow, Loading, Notice} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Focusable} from '../../ui/Focusable';
import {Icon, type IconName} from '../../ui/Icon';
import {ConfirmDialog, Sheet} from '../../ui/Sheet';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useSession} from '../../app/session';
import {useToast} from '../../app/toast';
import {push, root, back} from '../../nav/nav';
import {BlockView, FieldView, ActionButton, type Ctx} from './Blocks';

/** Server administration (owners and admins): every console section, rendered from the server's page blocks. */
export function AdminScreen() {
  useRegisterNav();
  const route = useRoute() as {params?: {section?: string; param?: string}};
  const m = useMetrics();
  const {isAdmin} = useSession();
  const admin = useAdmin();
  const chrome = useChromeInsets();
  const section = route.params?.section ?? (m.isPhone ? undefined : 'overview');
  if (!isAdmin) return <Screen><EmptyState icon="lock" title="Only the server owner can manage this server" body="Ask the owner to share what you need, or sign in with the owner account." action={{label: 'Go to Home', onPress: () => root('Home')}} /></Screen>;
  const groups = ['Console', 'Media', 'Live TV', 'Server'] as const;
  const nav = (
    <View style={{gap: 2}}>
      {groups.map(g => (
        <View key={g} style={{gap: 2, marginBottom: 8}}>
          <Text variant="label" tone="tertiary" style={{marginLeft: 14, marginVertical: 6}}>{g}</Text>
          {admin.sections.filter(s => s.group === g).map(s => {
            const active = s.id === section;
            return (
              <Focusable key={s.id} onPress={() => (m.isPhone ? push('Admin', {section: s.id}) : root('Admin', {section: s.id}))} onFocus={() => m.isTV && !active ? undefined : undefined} hasTVPreferredFocus={active && m.isTV} accessibilityLabel={s.label} accessibilityState={{selected: active}}
                style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: 12, paddingHorizontal: 14, height: m.isTV ? 58 : m.isPhone ? 50 : 40, borderRadius: m.radius.control, backgroundColor: focused ? colors.focus : active ? 'rgba(64,159,212,0.16)' : 'transparent'})}>
                {({focused}) => (
                  <>
                    <Icon name={s.icon as IconName} size={m.iconSmall + 2} color={focused ? colors.accentInk : active ? colors.accentStrong : colors.textSecondary} />
                    <Text variant={active ? 'bodyStrong' : 'body'} color={focused ? colors.accentInk : active ? colors.text : colors.textSecondary} numberOfLines={1} style={{flex: 1}}>{s.label}</Text>
                    {m.isPhone ? <Icon name="forward" size={16} color={colors.textTertiary} /> : null}
                  </>
                )}
              </Focusable>
            );
          })}
        </View>
      ))}
    </View>
  );
  if (m.isPhone && !section) {
    return (
      <Screen contentStyle={{paddingHorizontal: m.gutter, gap: m.space.md}}>
        <Text variant="label" tone="accent">Lighthouse</Text>
        <Text variant="display">Manage server</Text>
        {nav}
      </Screen>
    );
  }
  return (
    <View style={{flex: 1, flexDirection: 'row'}}>
      {!m.isPhone ? (
        <View style={{width: m.isTV ? 340 : 230, paddingTop: chrome.top, paddingLeft: m.isTV ? 24 : 12, borderRightWidth: 1, borderColor: colors.lineSoft}}>
          <ScrollView showsVerticalScrollIndicator={false} contentContainerStyle={{paddingBottom: chrome.bottom}}>
            <Text variant="label" tone="accent" style={{marginLeft: 14}}>Lighthouse</Text>
            <Text variant="heading" style={{marginLeft: 14, marginBottom: 10}}>Manage server</Text>
            {m.isTV ? <TVFocusGuideView autoFocus>{nav}</TVFocusGuideView> : nav}
          </ScrollView>
        </View>
      ) : null}
      <View style={{flex: 1}}>
        <AdminPageView key={`${section}-${route.params?.param ?? ''}`} section={section!} param={route.params?.param} />
      </View>
    </View>
  );
}

function AdminPageView({section, param}: {section: string; param?: string}) {
  const m = useMetrics();
  const admin = useAdmin();
  const toast = useToast();
  const chrome = useChromeInsets();
  const [page, setPage] = useState<AdminPage>();
  const [error, setError] = useState<string>();
  const [confirm, setConfirm] = useState<AdminAction>();
  const [form, setForm] = useState<{a: AdminAction; values: Record<string, unknown>}>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    const load = () => admin.load(section, param).then(p => alive && setPage(p), e => alive && setError(String(e)));
    void load();
    const off = admin.subscribe(() => void load());
    return () => { alive = false; off(); };
  }, [admin, section, param]);
  const run = async (a: AdminAction, input?: Record<string, unknown>) => {
    setBusy(true);
    const notice = await admin.act(a.id, a.target, input);
    setBusy(false);
    setConfirm(undefined);
    setForm(undefined);
    toast({message: notice ?? `${a.label}: done.`, icon: 'check'});
  };
  const ctx: Ctx = {
    onAction: a => {
      if (a.navigate) return a.navigate.section === section && !a.navigate.param ? undefined : push('Admin', {section: a.navigate.section, param: a.navigate.param});
      if (a.confirm) return setConfirm(a);
      if (a.form) return setForm({a, values: Object.fromEntries(a.form.fields.map(f => [f.key, f.value]))});
      void run(a);
    },
    onValue: (key, v) => { void admin.setValue(key, v); },
    onOpen: (s, p) => push('Admin', {section: s, param: p}),
  };
  const formCtx: Ctx = {...ctx, onValue: (key, v) => setForm(f => (f ? {...f, values: {...f.values, [key]: v}} : f))};
  if (error) return <Notice tone="error" body={error} />;
  const content = (
    <ScrollView contentContainerStyle={{paddingTop: chrome.top, paddingBottom: chrome.bottom, paddingHorizontal: m.isTV ? 48 : m.gutter, gap: m.space.lg, maxWidth: m.isTV ? 1500 : 1100}} showsVerticalScrollIndicator={!m.isTV}>
      {!page ? <Loading /> : (
        <>
          <View style={{flexDirection: m.isPhone ? 'column' : 'row', alignItems: m.isPhone ? 'flex-start' : 'flex-end', gap: m.space.md}}>
            <View style={{flex: m.isPhone ? undefined : 1, gap: 4}}>
              {page.back || m.isPhone ? <Button size="sm" icon="back" label={page.back?.label ?? 'Manage server'} variant="ghost" onPress={back} /> : null}
              <Text variant="display">{page.title}</Text>
              <Text variant="body" tone="secondary">{page.lede}</Text>
            </View>
            {page.actions?.length ? <View style={{flexDirection: 'row', gap: 8}}>{page.actions.map(a => <ActionButton key={a.id} a={a} ctx={ctx} />)}</View> : null}
          </View>
          {page.blocks.map((b, i) => <BlockView key={i} block={b} ctx={ctx} />)}
        </>
      )}
    </ScrollView>
  );
  return (
    <>
      {m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{content}</TVFocusGuideView> : content}
      <ConfirmDialog visible={!!confirm} onClose={() => setConfirm(undefined)} title={confirm?.confirm?.title ?? ''} body={confirm?.confirm?.body} confirmLabel={confirm?.confirm?.action ?? 'OK'} destructive={confirm?.tone === 'danger'} typeToConfirm={confirm?.confirm?.typeToConfirm} busy={busy} onConfirm={() => confirm && void run(confirm)} />
      <Sheet visible={!!form} onClose={() => setForm(undefined)} title={form?.a.form?.title} subtitle={form?.a.form?.lede} size="md" contentStyle={{gap: 0}}
        footer={<><Button label="Cancel" variant="ghost" onPress={() => setForm(undefined)} /><Button label={form?.a.form?.submit ?? 'Save'} variant={form?.a.tone === 'danger' ? 'danger' : 'primary'} loading={busy} onPress={() => form && void run(form.a, form.values)} /></>}>
        {form?.a.form?.fields.map(f => <FieldView key={f.key} f={{...f, value: form.values[f.key]} as AdminField} ctx={formCtx} local />)}
      </Sheet>
    </>
  );
}

export {ListRow};
