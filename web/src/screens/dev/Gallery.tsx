import {useState} from 'react';
import {Badge, Button, Card, Checkbox, Chip, Chips, ConfirmDialog, Dialog, Freshness, Grid, IconButton, Icon, iconNames, Input, KeyValue, ListRow, Loading, Menu, Metric, MetricGrid, Notice, Page, PageHeader, PasswordInput, ProgressBar, Section, Segmented, Select, SettingsActions, SettingsGroup, SettingsPage, SettingsRow, Shelf, Skeleton, StateView, Status, Surface, Switch, Table, tableCell, Tabs, Text, Inset, Stack, Row} from '../../ui';

/** Development-only catalogue of every primitive in every state. Deliberate i18n exception (M12):
 * sample copy here exercises the components, never ships to users, so its lines carry
 * `lint-strings-allow` instead of catalogue keys. */
export function GalleryScreen() {
  const [tab, setTab] = useState('discover');
  const [seg, setSeg] = useState<'grid' | 'list'>('grid');
  const [on, setOn] = useState(true);
  const [checked, setChecked] = useState(false);
  const [dialog, setDialog] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const sample = [1, 2, 3, 4, 5, 6, 7, 8];
  return (
    <Page>
      <PageHeader eyebrow="Design system" title="Gallery" subtitle="Every primitive in every state. Development only." actions={<><Button variant="ghost" label="Ghost" /><Button variant="primary" icon="play" label="Primary" /></>} /> {/* lint-strings-allow: developer-only screen */}
      <Section title="Type"> {/* lint-strings-allow: developer-only screen */}
        <Inset><Stack gap={6}>
          <Text as="p" variant="display">Display · The Grand Budapest Hotel</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="title">Title · Severance</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="heading">Heading · Continue watching</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="subheading">Subheading · Season 2</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="body">Body · A restrained blue atmosphere prevents product pages from collapsing into empty black.</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="caption" tone="secondary">Caption secondary · 2h 14m · PG-13 · Drama</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="micro" tone="tertiary">Micro tertiary · Checked 2 minutes ago</Text> {/* lint-strings-allow: developer-only screen */}
          <Text as="p" variant="label" tone="tertiary">Label · Playback</Text> {/* lint-strings-allow: developer-only screen */}
        </Stack></Inset>
      </Section>
      <Section title="Buttons"> {/* lint-strings-allow: developer-only screen */}
        <Inset><Stack>
          <Row><Button variant="primary" label="Primary" /><Button variant="secondary" label="Secondary" /><Button variant="outline" label="Outline" /><Button variant="ghost" label="Ghost" /><Button variant="danger" label="Danger" /><Button variant="glass" label="Glass" /><Button variant="link" label="Link" /></Row> {/* lint-strings-allow: developer-only screen */}
          <Row><Button variant="primary" icon="play" label="Play" size="lg" /><Button variant="secondary" icon="bookmark" label="My List" /><Button variant="secondary" icon="plus" label="Playlist" size="sm" /><Button variant="primary" label="Saving" loading /><Button variant="secondary" label="Disabled" disabled /><Button variant="secondary" label="Selected" selected /></Row> {/* lint-strings-allow: developer-only screen */}
          <Row><IconButton name="more" label="More" /><IconButton name="heart" label="Favourite" variant="ghost" /><IconButton name="close" label="Close" variant="outline" round /><IconButton name="play" label="Play" variant="primary" round size="lg" /></Row> {/* lint-strings-allow: developer-only screen */}
        </Stack></Inset>
      </Section>
      <Section title="Tabs and choices"> {/* lint-strings-allow: developer-only screen */}
        <Tabs items={[{id: 'discover', label: 'Discover'}, {id: 'browse', label: 'Movies', count: 412}, {id: 'collections', label: 'Collections'}, {id: 'categories', label: 'Categories'}, {id: 'off', label: 'Unavailable', disabled: true}]} value={tab} onChange={setTab} /> {/* lint-strings-allow: developer-only screen */}
        <Inset><Row>
          <Segmented options={[{id: 'grid', icon: 'grid', label: 'Grid'}, {id: 'list', icon: 'list', label: 'List'}]} value={seg} onChange={setSeg} label="View" /> {/* lint-strings-allow: developer-only screen */}
          <Chips><Chip label="Recently added" pressed /><Chip label="Unwatched" /><Chip label="4K" count={12} /><Chip label="Sort: Title" icon="sort" /></Chips> {/* lint-strings-allow: developer-only screen */}
          <Menu trigger={<Button variant="outline" icon="more" label="Menu" />} items={[{id: 'a', label: 'Play', icon: 'play'}, {id: 'b', label: 'Add to My List', icon: 'bookmark'}, {id: 'c', label: 'Mark watched', icon: 'check', selected: true}, {id: 'd', label: 'Remove', icon: 'trash', destructive: true, separatorBefore: true}]} onSelect={() => {}} /> {/* lint-strings-allow: developer-only screen */}
        </Row></Inset>
      </Section>
      <Section title="Cards"> {/* lint-strings-allow: developer-only screen */}
        <Shelf title="Poster shelf" count={8} action={{label: 'See all', onClick: () => {}}}> {/* lint-strings-allow: developer-only screen */}
          {sample.map(i => <Card key={i} title={`Movie ${i}`} caption="2024 · 1h 58m" progress={i === 2 ? 0.4 : undefined} watched={i === 3} onOpen={() => {}} onPlay={() => {}} onMore={() => {}} />)} {/* lint-strings-allow: developer-only screen */}
        </Shelf>
        <Shelf title="Square shelf" density="square"> {/* lint-strings-allow: developer-only screen */}
          {sample.map(i => <Card key={i} title={`Album ${i}`} caption="Artist" shape="square" icon="music" onOpen={() => {}} onPlay={() => {}} />)} {/* lint-strings-allow: developer-only screen */}
        </Shelf>
        <Shelf title="People" density="person"> {/* lint-strings-allow: developer-only screen */}
          {sample.map(i => <Card key={i} title={`Person ${i}`} caption="Director" shape="circle" onOpen={() => {}} />)} {/* lint-strings-allow: developer-only screen */}
        </Shelf>
        <Grid density="poster">{sample.map(i => <Card key={i} title={`Grid ${i}`} caption="2023" onOpen={() => {}} unavailable={i === 5} />)}</Grid> {/* lint-strings-allow: developer-only screen */}
      </Section>
      <Section title="Rows"> {/* lint-strings-allow: developer-only screen */}
        <Inset><Surface padless>
          <ListRow index={1} title="Pilot" subtitle="S1 · E1 · 58m" meta="Watched" onClick={() => {}} /> {/* lint-strings-allow: developer-only screen */}
          <ListRow index={2} title="Half Loop" subtitle="S1 · E2 · 47m" onClick={() => {}} trailing={<ProgressBar value={0.3} thin />} /> {/* lint-strings-allow: developer-only screen */}
          <ListRow art={{path: undefined, icon: 'tv'}} artShape="landscape" title="In Perpetuity" subtitle="S1 · E3 · 52m · A revealing tour of the company’s past." meta="52m" onClick={() => {}} /> {/* lint-strings-allow: developer-only screen */}
          <ListRow icon="server" title="Justin’s Mac" subtitle="Through your Portico Account" trailingIcon="forward" onClick={() => {}} selected /> {/* lint-strings-allow: developer-only screen */}
        </Surface></Inset>
      </Section>
      <Section title="Fields"> {/* lint-strings-allow: developer-only screen */}
        <Inset><div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 16}}>
          <Input label="Server address" placeholder="https://portico.example.com" help="HTTPS is required outside your home network." /> {/* lint-strings-allow: developer-only screen */}
          <Input label="Username" defaultValue="justin" error="This username is already taken." /> {/* lint-strings-allow: developer-only screen */}
          <PasswordInput label="Password" defaultValue="hunter22" /> {/* lint-strings-allow: developer-only screen */}
          <Input label="Activation code" code defaultValue="5QDC-E2YR" /> {/* lint-strings-allow: developer-only screen */}
          <Select label="Poster size" options={[{value: 'compact', label: 'Compact'}, {value: 'regular', label: 'Regular'}, {value: 'large', label: 'Large'}]} defaultValue="regular" /> {/* lint-strings-allow: developer-only screen */}
          <Stack gap={10}><Row><Switch checked={on} onCheckedChange={setOn} label="Autoplay" /><Text variant="body">Autoplay next episode</Text></Row><Checkbox checked={checked} onCheckedChange={setChecked} label="Remember this profile on this browser" help="A remembered selection never extends server permissions." /></Stack> {/* lint-strings-allow: developer-only screen */}
        </div></Inset>
      </Section>
      <Section title="Feedback"> {/* lint-strings-allow: developer-only screen */}
        <Inset><Stack>
          <Notice tone="info" title="Manage your server on the web">Libraries, sharing and remote access live in Portico for the web.</Notice> {/* lint-strings-allow: developer-only screen */}
          <Notice tone="success">Library scan finished. 214 items added.</Notice> {/* lint-strings-allow: developer-only screen */}
          <Notice tone="warning" action={{label: 'Check again', onClick: () => {}}}>This observation is older than five minutes.</Notice> {/* lint-strings-allow: developer-only screen */}
          <Notice tone="error" title="Couldn’t reach the server" action={{label: 'Try again', onClick: () => {}}} secondaryAction={{label: 'Choose another server', onClick: () => {}}}>Your saved connection did not respond.</Notice> {/* lint-strings-allow: developer-only screen */}
          <Row><Badge>Neutral</Badge><Badge tone="accent">4K HDR</Badge><Badge tone="healthy" dot>Healthy</Badge><Badge tone="warning" dot>Attention</Badge><Badge tone="danger" dot>Failed</Badge><Badge tone="record" live>Recording</Badge><Badge outline>Outline</Badge></Row> {/* lint-strings-allow: developer-only screen */}
          <Row><Status tone="healthy">Connected</Status><Status tone="warning">Degraded</Status><Status tone="danger">Offline</Status><Status tone="neutral">Idle</Status><Freshness at={Date.now() - 90_000} onRefresh={() => {}} /><Freshness at={Date.now() - 900_000} onRefresh={() => {}} /></Row> {/* lint-strings-allow: developer-only screen */}
          <ProgressBar value={0.62} label="Scan" /><ProgressBar indeterminate label="Working" /><Loading label="Loading libraries…" /> {/* lint-strings-allow: developer-only screen */}
          <Row><Skeleton width={160} height={240} /><Stack gap={8}><Skeleton width={220} height={18} /><Skeleton width={140} height={14} /><Skeleton width={320} height={14} /></Stack></Row>
          <Surface><StateView icon="search" title="No results for “severence”" body="Check the spelling or try a shorter query." action={{label: 'Clear search', onClick: () => {}}} /></Surface> {/* lint-strings-allow: developer-only screen */}
          <MetricGrid><Metric label="Active streams" value="3" sub="2 direct · 1 transcoding" /><Metric label="CPU" value="41%" sub="8 cores" /><Metric label="Storage" value="1.2 TB" sub="of 4 TB used" /><Metric label="Database" value="182 MB" /></MetricGrid> {/* lint-strings-allow: developer-only screen */}
          <KeyValue rows={[['Server', 'Justin’s Mac'], ['Version', '0.1.0-dev'], ['Route', 'Local network']]} />
        </Stack></Inset>
      </Section>
      <Section title="Settings primitives"> {/* lint-strings-allow: developer-only screen */}
        <Inset><SettingsPage>
          <SettingsGroup title="Playback" description="Choices that shape how media plays on this server."> {/* lint-strings-allow: developer-only screen */}
            <SettingsRow label="Server name" help="Shown on every device that connects." control={<Input label="Server name" defaultValue="Justin’s Mac" className="visually-hidden-label" />} /> {/* lint-strings-allow: developer-only screen */}
            <SettingsRow label="Allow transcoding" help="Convert media for devices that cannot play the original." control={<Switch checked={on} onCheckedChange={setOn} label="Allow transcoding" />} state={<><Icon name="check" size={12} /> Saved · takes effect on the next stream</>} /> {/* lint-strings-allow: developer-only screen */}
            <SettingsRow label="Maximum simultaneous streams" meta="8" onClick={() => {}} /> {/* lint-strings-allow: developer-only screen */}
            <SettingsRow label="Danger zone" help="Remove this library and its records. Files are not deleted." control={<Button variant="danger" label="Remove library" onClick={() => setConfirm(true)} />} /> {/* lint-strings-allow: developer-only screen */}
          </SettingsGroup>
          <SettingsActions note="3 unsaved changes"><Button variant="ghost" label="Discard" /><Button variant="primary" label="Save changes" /></SettingsActions> {/* lint-strings-allow: developer-only screen */}
          <Table columns={[{label: 'Job'}, {label: 'State'}, {label: 'Started'}, {label: 'Progress', num: true}, {label: '', actions: true}]}> {/* lint-strings-allow: developer-only screen */}
            <tr><td>Scan · Movies</td><td><Status tone="accent">Running</Status></td><td>2 min ago</td><td className={tableCell.num}>62%</td><td className={tableCell.actions}><Button size="sm" variant="ghost" label="Cancel" /></td></tr> {/* lint-strings-allow: developer-only screen */}
            <tr><td>Metadata refresh · TV</td><td><Status tone="healthy">Done</Status></td><td>1 h ago</td><td className={tableCell.num}>100%</td><td className={tableCell.actions}><Button size="sm" variant="ghost" label="Details" /></td></tr> {/* lint-strings-allow: developer-only screen */}
            <tr><td>Backup</td><td><Status tone="danger">Failed</Status></td><td>Yesterday</td><td className={tableCell.num}>—</td><td className={tableCell.actions}><Button size="sm" variant="ghost" label="Try again" /></td></tr> {/* lint-strings-allow: developer-only screen */}
          </Table>
        </SettingsPage></Inset>
      </Section>
      <Section title="Overlays"> {/* lint-strings-allow: developer-only screen */}
        <Inset><Row><Button label="Open dialog" onClick={() => setDialog(true)} /><Button variant="danger" label="Open confirmation" onClick={() => setConfirm(true)} /></Row></Inset> {/* lint-strings-allow: developer-only screen */}
        <Dialog open={dialog} onOpenChange={setDialog} title="Switch profile" description="Justin’s Mac" actions={<><Button variant="ghost" label="Cancel" onClick={() => setDialog(false)} /><Button variant="primary" label="Use this profile" onClick={() => setDialog(false)} /></>}> {/* lint-strings-allow: developer-only screen */}
          <ListRow icon="profile" title="Justin" subtitle="Primary profile" onClick={() => {}} selected /> {/* lint-strings-allow: developer-only screen */}
          <ListRow icon="profile" title="Kids" subtitle="PIN protected" onClick={() => {}} /> {/* lint-strings-allow: developer-only screen */}
        </Dialog>
        <ConfirmDialog open={confirm} onOpenChange={setConfirm} title="Remove “Movies”?" body="Portico forgets this library, its metadata and watch history. Media files on disk are not touched. 1 scan is running and will be cancelled." confirmLabel="Remove library" typedConfirmation="Movies" onConfirm={() => setConfirm(false)} /> {/* lint-strings-allow: developer-only screen */}
      </Section>
      <Section title="Icons"> {/* lint-strings-allow: developer-only screen */}
        <Inset><div style={{display: 'flex', flexWrap: 'wrap', gap: 16}}>{iconNames.map(n => <span key={n} title={n} style={{display: 'grid', placeItems: 'center', width: 40, height: 40, borderRadius: 8, background: 'var(--surface-translucent)'}}><Icon name={n} /></span>)}</div></Inset>
      </Section>
    </Page>
  );
}
