import type { MediaItem } from './models';

export type Media = MediaItem;

export const media: Media[] = [
  {
    id: 'fargo', title: 'Fargo', subtitle: 'The Castle', year: 2015, type: 'show', kind: 'show', rating: 'TV-MA', length: '48m', genre: 'Crime drama', progress: 62,
    poster: 'https://image.tmdb.org/t/p/w780/a3VW6khsyUVKrG0GBCWFG3NzWPX.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/4jrSbRpLqpvYJtLKncaxZVC47EW.jpg',
  },
  {
    id: 'rookie', title: 'The Rookie', subtitle: 'Survive the Streets', year: 2018, type: 'show', kind: 'show', rating: 'TV-14', length: '43m', genre: 'Drama', progress: 19,
    poster: 'https://image.tmdb.org/t/p/w780/70kTz0OmjjZe7zHvIDrq2iKW7PJ.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/6iNWfGVCEfASDdlNb05TP5nG0ll.jpg',
  },
  {
    id: 'hurt-locker', title: 'The Hurt Locker', subtitle: '2008', year: 2008, type: 'movie', kind: 'movie', rating: 'R', length: '2h 11m', genre: 'Thriller',
    poster: 'https://image.tmdb.org/t/p/w780/io2dfBJhasvGbgkCX9cCGVOiA99.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/nKieVGBCZQfcylwO7mOMPaug8f2.jpg',
  },
  {
    id: 'dolphin-reef', title: 'Dolphin Reef', subtitle: '2018', year: 2018, type: 'movie', kind: 'movie', rating: 'G', length: '1h 17m', genre: 'Documentary',
    poster: 'https://image.tmdb.org/t/p/w780/hjXDxftBdM4mdhcO3eTqJCUt3Y8.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/ySX3SKhiVjs5phEGrIzEqbmp9YR.jpg',
  },
  {
    id: 'earth-stood-still', title: 'The Day the Earth Stood Still', subtitle: '2008', year: 2008, type: 'movie', kind: 'movie', rating: 'PG-13', length: '1h 44m', genre: 'Science fiction',
    poster: 'https://image.tmdb.org/t/p/w780/vBgFSYmG5tb7GsZ3tHR0WNaWaxA.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/nIxFtd6gcmGGvFxyb1z3mt46CrN.jpg',
  },
  {
    id: 'martian', title: 'The Martian', subtitle: '2015', year: 2015, type: 'movie', kind: 'movie', rating: 'PG-13', length: '2h 24m', genre: 'Science fiction',
    poster: 'https://image.tmdb.org/t/p/w780/fASz8A0yFE3QB6LgGoOfwvFSseV.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/lzMS0CI3FLQYC5EgJoWeIaEt0lm.jpg',
  },
  {
    id: 'blade-runner', title: 'Blade Runner 2049', subtitle: '2017', year: 2017, type: 'movie', kind: 'movie', rating: 'R', length: '2h 44m', genre: 'Science fiction',
    poster: 'https://image.tmdb.org/t/p/w780/gajva2L0rPYkEWjzgFlBXCAVBE5.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/ilRyazdMJwN05exqhwK4tMKBYZs.jpg',
  },
  {
    id: 'life-aquatic', title: 'The Life Aquatic', subtitle: '2004', year: 2004, type: 'movie', kind: 'movie', rating: 'R', length: '1h 59m', genre: 'Comedy',
    poster: 'https://image.tmdb.org/t/p/w780/qZoFLNBC78jzboWeDH6Ha0qavF2.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/7tWktnLQ81G8pHUkhuTebcTkcB3.jpg',
  },
  {
    id: 'florida-project', title: 'The Florida Project', subtitle: '2017', year: 2017, type: 'movie', kind: 'movie', rating: 'R', length: '1h 51m', genre: 'Drama',
    poster: 'https://image.tmdb.org/t/p/w780/5QnDxdJg1fi6uMSkSi4x8tHsltm.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/lO5c0zG8s1dBPHcRparpNlNGWU9.jpg',
  },
  {
    id: 'run-lola-run', title: 'Run Lola Run', subtitle: '1998', year: 1998, type: 'movie', kind: 'movie', rating: 'R', length: '1h 20m', genre: 'Thriller',
    poster: 'https://image.tmdb.org/t/p/w780/v0giIi4bTILVhNhJajet3WWY3FA.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/wRcm3iwi4BeMN59U4fjO7u3dOVI.jpg',
  },
  {
    id: 'panic-room', title: 'Panic Room', subtitle: '2002', year: 2002, type: 'movie', kind: 'movie', rating: 'R', length: '1h 52m', genre: 'Thriller',
    poster: 'https://image.tmdb.org/t/p/w780/hANYbvfwxmkC9E4yY6YyJxYxlSJ.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/9G5bteYI8FXaY9EomnUP0kdHm66.jpg',
  },
  {
    id: 'grease', title: 'Grease', subtitle: '1978', year: 1978, type: 'movie', kind: 'movie', rating: 'PG', length: '1h 50m', genre: 'Musical',
    poster: 'https://image.tmdb.org/t/p/w780/2rM7fQKpb7cs1Iq7IBqub9LFDzJ.jpg',
    backdrop: 'https://image.tmdb.org/t/p/w1280/pdhDFmVQSA0f5i5IL0gpWROjgZ5.jpg',
  },
];

const musicArtwork = 'https://images.unsplash.com/photo-1619983081563-430f63602796?auto=format&fit=crop&w=900&q=88';
const musicBackdrop = 'https://images.unsplash.com/photo-1496293455970-f8581aae0e3b?auto=format&fit=crop&w=1800&q=88';

export const music = {
  artist: 'Bonobo',
  album: 'Black Sands',
  year: 2010,
  artwork: musicArtwork,
  backdrop: musicBackdrop,
  tracks: [
    ['Prelude', '1:18'], ['Kiara', '3:50'], ['Kong', '3:58'], ['Eyesdown', '5:28'],
    ['El Toro', '3:44'], ['We Could Forever', '4:20'], ['1009', '4:31'], ['All in Forms', '4:51'],
  ],
};

export const musicMedia: Media[] = [
  { id: 'artist-bonobo', title: 'Bonobo', subtitle: '7 albums · 94 tracks', year: 2000, type: 'music', kind: 'artist', rating: '', length: '', genre: 'Electronic', poster: musicArtwork, backdrop: musicBackdrop },
  { id: 'artist-tycho', title: 'Tycho', subtitle: '6 albums · 72 tracks', year: 2002, type: 'music', kind: 'artist', rating: '', length: '', genre: 'Ambient', poster: 'https://images.unsplash.com/photo-1524368535928-5b5e00ddc76b?auto=format&fit=crop&w=900&q=80', backdrop: musicBackdrop },
  { id: 'artist-little-dragon', title: 'Little Dragon', subtitle: '5 albums · 61 tracks', year: 1996, type: 'music', kind: 'artist', rating: '', length: '', genre: 'Electronic', poster: 'https://images.unsplash.com/photo-1506157786151-b8491531f063?auto=format&fit=crop&w=900&q=86', backdrop: musicBackdrop },
  { id: 'album-black-sands', title: 'Black Sands', subtitle: 'Bonobo · 2010', year: 2010, type: 'music', kind: 'album', rating: '', length: '47m', genre: 'Electronic', poster: musicArtwork, backdrop: musicBackdrop, parentTitle: 'Bonobo' },
  { id: 'album-dive', title: 'Dive', subtitle: 'Tycho · 2011', year: 2011, type: 'music', kind: 'album', rating: '', length: '50m', genre: 'Ambient', poster: 'https://images.unsplash.com/photo-1614613535308-eb5fbd3d2c17?auto=format&fit=crop&w=800&q=86', backdrop: musicBackdrop, parentTitle: 'Tycho' },
  { id: 'album-ritual-union', title: 'Ritual Union', subtitle: 'Little Dragon · 2011', year: 2011, type: 'music', kind: 'album', rating: '', length: '42m', genre: 'Electronic', poster: 'https://images.unsplash.com/photo-1598387993281-cecf8b71a8f8?auto=format&fit=crop&w=800&q=86', backdrop: musicBackdrop, parentTitle: 'Little Dragon' },
  { id: 'track-kiara', title: 'Kiara', subtitle: 'Bonobo · Black Sands', year: 2010, type: 'music', kind: 'track', rating: '', length: '3:50', genre: 'Electronic', poster: musicArtwork, backdrop: musicBackdrop, parentTitle: 'Black Sands' },
  { id: 'track-kong', title: 'Kong', subtitle: 'Bonobo · Black Sands', year: 2010, type: 'music', kind: 'track', rating: '', length: '3:58', genre: 'Electronic', poster: musicArtwork, backdrop: musicBackdrop, parentTitle: 'Black Sands' },
  { id: 'track-eyesdown', title: 'Eyesdown', subtitle: 'Bonobo · Black Sands', year: 2010, type: 'music', kind: 'track', rating: '', length: '5:28', genre: 'Electronic', poster: musicArtwork, backdrop: musicBackdrop, parentTitle: 'Black Sands' },
];

export const channels = [
  { id: 'kanal', number: '1', name: 'Kanal 7', call: 'K7', now: 'Evening News', next: 'The Agenda', color: '#5d8bb4' },
  { id: 'a2', number: '2', name: 'A2 CNN Albania', call: 'A2', now: 'A2 Business', next: 'World View', color: '#a64242' },
  { id: 'abc', number: '3', name: 'ABC News', call: 'ABC', now: 'ABC News Live', next: 'Prime', color: '#54677e' },
  { id: 'bbc', number: '4', name: 'BBC World News', call: 'BBC', now: 'Global', next: 'The Context', color: '#8c3434' },
];

export const serverSettings = [
  ['status', 'Status'], ['general', 'General'], ['media', 'Media'], ['playback', 'Playback'], ['live', 'Live TV & DVR'],
  ['connectivity', 'Connectivity'], ['people', 'People & Access'], ['maintenance', 'Maintenance'], ['diagnostics', 'Diagnostics'],
] as const;

export const personalSettings = [
  ['account', 'Account'], ['appearance', 'Appearance'], ['personal-playback', 'Playback'], ['privacy', 'Privacy'],
] as const;
