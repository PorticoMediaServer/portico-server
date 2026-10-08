import type { HomeVideo, Memory, Photo, PhotoAlbum } from '../types.ts';
import { art, daysAgo, paletteFor, rng } from './util.ts';

interface AlbumDef {
  id: string;
  title: string;
  kind: PhotoAlbum['kind'];
  place: string;
  daysAgo: number;
  spanDays: number;
  camera: string;
  lens: string;
  seeds: number[];
  portrait?: number[];
}

export const photoAlbumDefs: AlbumDef[] = [
  {
    id: 'iceland',
    title: 'Iceland',
    kind: 'trip',
    place: 'South Coast, Iceland',
    daysAgo: 590,
    spanDays: 9,
    camera: 'Fujifilm X100VI',
    lens: '23mm f/2',
    seeds: [6, 182, 240, 273, 337, 379, 146, 584, 266, 438, 577, 423, 2, 357, 579, 551],
    portrait: [273, 584],
  },
  {
    id: 'amalfi',
    title: 'Amalfi & Cinque Terre',
    kind: 'trip',
    place: 'Liguria & Campania, Italy',
    daysAgo: 385,
    spanDays: 11,
    camera: 'iPhone 16 Pro',
    lens: 'Main camera 24mm f/1.78',
    seeds: [243, 127, 502, 441, 8, 23, 418, 252, 76, 425, 426, 432, 433],
    portrait: [418, 252],
  },
  {
    id: 'new-york',
    title: 'New York at Christmas',
    kind: 'trip',
    place: 'New York, NY',
    daysAgo: 290,
    spanDays: 5,
    camera: 'iPhone 16 Pro',
    lens: 'Main camera 24mm f/1.78',
    seeds: [70, 180, 188, 187, 201, 256, 257, 295, 497, 340, 382, 220, 100, 163, 118, 260],
    portrait: [100, 295, 497],
  },
  {
    id: 'winter-cabin',
    title: 'Winter Cabin',
    kind: 'trip',
    place: 'Jay Peak, Vermont',
    daysAgo: 250,
    spanDays: 4,
    camera: 'Sony α7 IV',
    lens: 'FE 35mm f/1.4 GM',
    seeds: [132, 213, 121, 400, 575, 31, 112, 198, 291, 294],
    portrait: [121],
  },
  {
    id: 'san-francisco',
    title: 'San Francisco',
    kind: 'trip',
    place: 'San Francisco, CA',
    daysAgo: 160,
    spanDays: 4,
    camera: 'iPhone 16 Pro',
    lens: 'Telephoto 120mm f/2.8',
    seeds: [130, 166, 110, 171, 292, 566, 139, 164],
  },
  {
    id: 'paris',
    title: 'Paris Weekend',
    kind: 'trip',
    place: 'Paris, France',
    daysAgo: 100,
    spanDays: 3,
    camera: 'Fujifilm X100VI',
    lens: '23mm f/2',
    seeds: [545, 553, 592, 86, 329, 131],
    portrait: [592],
  },
  {
    id: 'maine-summer',
    title: 'Maine, Summer 2026',
    kind: 'trip',
    place: 'Popham Beach, Maine',
    daysAgo: 70,
    spanDays: 14,
    camera: 'iPhone 16 Pro',
    lens: 'Main camera 24mm f/1.78',
    seeds: [37, 40, 41, 55, 312, 77, 483, 465, 88, 162, 134, 303],
    portrait: [162],
  },
  {
    id: 'home-garden',
    title: 'Home & Garden',
    kind: 'place',
    place: 'Home',
    daysAgo: 200,
    spanDays: 185,
    camera: 'iPhone 16 Pro',
    lens: 'Main camera 24mm f/1.78',
    seeds: [0, 109, 115, 140, 200, 424, 407, 543, 14, 269, 178, 499, 84, 176, 239, 186, 516, 471, 13, 165],
    portrait: [109, 471],
  },
];

const EXPO = ['1/250s · f/2.8 · ISO 160', '1/1000s · f/4 · ISO 100', '1/60s · f/2 · ISO 800', '1/125s · f/5.6 · ISO 200', '1/500s · f/8 · ISO 100', '1/30s · f/1.8 · ISO 1600', '4s · f/2 · ISO 3200'];

export const photos: Photo[] = photoAlbumDefs.flatMap((a) => {
  const r = rng(a.id);
  return a.seeds.map((seed, i) => {
    const portrait = a.portrait?.includes(seed) ?? false;
    const id = `ph-${seed}`;
    const d = Math.max(0.5, a.daysAgo - (a.spanDays * i) / a.seeds.length);
    const thumb = art.photoThumb(id);
    return {
      id,
      kind: 'photo' as const,
      title: `IMG_${4000 + seed * 3}`,
      src: art.photo(id),
      thumb,
      width: portrait ? 1067 : 1600,
      height: portrait ? 1600 : 1067,
      takenAt: daysAgo(d, 8 + Math.floor(r() * 12)),
      albumIds: [a.id],
      place: a.place,
      camera: a.camera,
      lens: a.lens,
      exposure: EXPO[Math.floor(r() * EXPO.length)],
      favorite: r() > 0.78,
      palette: paletteFor(thumb),
    };
  });
});

photos.sort((a, b) => Date.parse(b.takenAt) - Date.parse(a.takenAt));

export const photoAlbums: PhotoAlbum[] = [
  ...photoAlbumDefs.map((a) => {
    const ps = photos.filter((p) => p.albumIds.includes(a.id));
    const start = new Date(ps[ps.length - 1].takenAt);
    const end = new Date(ps[0].takenAt);
    const fmt = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    return {
      id: a.id,
      title: a.title,
      subtitle: a.kind === 'place' ? `${ps.length} photos` : `${fmt(start)} – ${fmt(end)} · ${ps.length} photos`,
      cover: art.photoThumb(`ph-${a.seeds[0]}`),
      photoIds: ps.map((p) => p.id),
      kind: a.kind,
    };
  }),
  {
    id: 'favorites',
    title: 'Favorites',
    subtitle: 'Smart album',
    cover: photos.find((p) => p.favorite)!.thumb,
    photoIds: photos.filter((p) => p.favorite).map((p) => p.id),
    kind: 'smart',
  },
];

export const memories: Memory[] = [
  { id: 'mem-iceland', title: 'Iceland', subtitle: 'Ring Road, winter', album: 'iceland' },
  { id: 'mem-maine', title: 'Summer in Maine', subtitle: 'Popham Beach · 2026', album: 'maine-summer' },
  { id: 'mem-nyc', title: 'Christmas in New York', subtitle: 'Last December', album: 'new-york' },
  { id: 'mem-italy', title: 'One Year Ago', subtitle: 'Amalfi & Cinque Terre', album: 'amalfi' },
].map((m) => {
  const ps = photoAlbums.find((a) => a.id === m.album)!;
  return { id: m.id, title: m.title, subtitle: m.subtitle, cover: ps.cover.replace('/photo-thumb/', '/photo/'), photoIds: ps.photoIds };
});

interface HomeVideoDef {
  id: string;
  title: string;
  seed: number;
  daysAgo: number;
  duration: string;
  place: string;
  device: string;
  summary: string;
  people: string[];
}

export const homeVideoDefs: HomeVideoDef[] = [
  { id: 'hv-bike', title: 'Theo’s First Bike Ride', seed: 178, daysAgo: 35, duration: '2:48', place: 'Riverside Park', device: 'iPhone 16 Pro', summary: 'No training wheels, three falls, one very proud rider.', people: ['Theo', 'Daniel'] },
  { id: 'hv-drone', title: 'Drone over Þingvellir', seed: 579, daysAgo: 588, duration: '4:12', place: 'Þingvellir, Iceland', device: 'DJI Mini 4 Pro', summary: 'Aerial footage over the rift valley on the one clear morning of the trip.', people: [] },
  { id: 'hv-balloon', title: 'Balloon Ride at Sunrise', seed: 203, daysAgo: 470, duration: '6:31', place: 'Göreme, Türkiye', device: 'iPhone 15 Pro', summary: 'Up at 4:30 a.m. for this. Worth it.', people: ['Maya', 'Daniel'] },
  { id: 'hv-fireworks', title: 'New Year’s Fireworks', seed: 589, daysAgo: 270, duration: '3:05', place: 'Portland, Maine', device: 'iPhone 16 Pro', summary: 'Midnight from the Eastern Prom. Theo made it to 11:58.', people: ['Theo', 'Noor', 'Maya', 'Daniel'] },
  { id: 'hv-beach', title: 'Beach Day at Popham', seed: 303, daysAgo: 66, duration: '5:20', place: 'Popham Beach, Maine', device: 'iPhone 16 Pro', summary: 'Sandcastles, one lost shovel and a seal sighting.', people: ['Theo', 'Daniel'] },
  { id: 'hv-sledding', title: 'Sledding at the Cabin', seed: 121, daysAgo: 248, duration: '4:44', place: 'Jay Peak, Vermont', device: 'Sony α7 IV', summary: 'The big hill behind the cabin, twelve runs in a row.', people: ['Theo', 'Noor'] },
  { id: 'hv-mormor', title: 'Mormor’s 80th Birthday', seed: 131, daysAgo: 140, duration: '12:36', place: 'Göteborg, Sweden', device: 'iPhone 16 Pro', summary: 'Speeches, singing and Mormor’s famous cardamom buns. Grandma Ingrid’s toast starts at 4:10.', people: ['Ingrid', 'Maya', 'Noor', 'Theo'] },
  { id: 'hv-paris', title: 'Walking the Seine', seed: 545, daysAgo: 99, duration: '3:58', place: 'Paris, France', device: 'Fujifilm X100VI', summary: 'An evening walk from the Pont Neuf to the Eiffel Tower.', people: ['Maya', 'Daniel'] },
  { id: 'hv-garden', title: 'Garden Olympics 2026', seed: 13, daysAgo: 48, duration: '8:02', place: 'Home', device: 'iPhone 16 Pro', summary: 'Egg-and-spoon, sack race and a disputed long jump.', people: ['Theo', 'Noor', 'Sam'] },
  { id: 'hv-skogafoss', title: 'Skógafoss in the Rain', seed: 273, daysAgo: 587, duration: '1:51', place: 'Skógafoss, Iceland', device: 'Fujifilm X100VI', summary: 'Soaked, but there was a rainbow.', people: ['Maya'] },
  { id: 'hv-lagoon', title: 'Blue Lagoon', seed: 182, daysAgo: 584, duration: '2:22', place: 'Grindavík, Iceland', device: 'iPhone 15 Pro', summary: 'Steam, silica masks and a very happy Noor.', people: ['Noor', 'Maya'] },
  { id: 'hv-school-concert', title: 'Noor’s Spring Concert', seed: 401, daysAgo: 150, duration: '18:40', place: 'Deering High School', device: 'Sony α7 IV', summary: 'Jazz band, second piece. Noor’s trumpet solo at 6:15.', people: ['Noor'] },
];

export const homeVideos: HomeVideo[] = homeVideoDefs.map((d) => {
  const [m, s] = d.duration.split(':').map(Number);
  const thumb = art.still(d.id);
  return {
    id: d.id,
    kind: 'homevideo',
    title: d.title,
    recordedAt: daysAgo(d.daysAgo, 15),
    duration: m * 60 + s,
    place: d.place,
    device: d.device,
    summary: d.summary,
    people: d.people,
    libraryId: 'lib-homevideos',
    art: { thumb, backdrop: thumb },
    palette: paletteFor(thumb),
    addedAt: daysAgo(Math.max(0, d.daysAgo - 1), 20),
    streamId: 'coffee-run',
  };
});
