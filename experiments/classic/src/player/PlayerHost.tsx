import React from 'react';
import {Modal} from 'react-native';
import {FocusGate} from '../ui/focusGate';
import {usePlayer, usePlayerState} from './PlayerProvider';
import {VideoPlayer} from './VideoPlayer';
import {NowPlaying} from './NowPlaying';

/** Shows the full-screen player for whatever is playing (Menu or swipe down closes it). */
export function PlayerHost() {
  const s = usePlayerState();
  const player = usePlayer();
  const visible = s.mode !== 'none' && s.expanded;
  const video = s.mode === 'video' || s.mode === 'live';
  return (
    <Modal visible={visible} animationType={video ? 'fade' : 'slide'} presentationStyle="fullScreen" supportedOrientations={['portrait', 'landscape', 'landscape-left', 'landscape-right']} onRequestClose={() => (video ? player.close() : player.minimize())}>
      {visible ? <FocusGate key={video ? "video" : "audio"}>{video ? <VideoPlayer /> : <NowPlaying />}</FocusGate> : null}
    </Modal>
  );
}
