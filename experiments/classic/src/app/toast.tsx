import React, {createContext, useCallback, useContext, useEffect, useRef, useState} from 'react';
import {Animated, View} from 'react-native';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Text} from '../ui/Text';
import {Icon, type IconName} from '../ui/Icon';
import {Focusable} from '../ui/Focusable';

type Toast = {id: number; message: string; icon?: IconName; tone?: 'info' | 'error' | 'success'; action?: {label: string; onPress: () => void}};
const Ctx = createContext<(t: Omit<Toast, 'id'>) => void>(() => {});

/** Short confirmations ("Added to Watchlist") with an optional Undo. */
export function ToastProvider({children}: {children: React.ReactNode}) {
  const [toast, setToast] = useState<Toast>();
  const show = useCallback((t: Omit<Toast, 'id'>) => setToast({...t, id: Date.now()}), []);
  return (
    <Ctx.Provider value={show}>
      {children}
      {toast ? <ToastView key={toast.id} toast={toast} onDone={() => setToast(undefined)} /> : null}
    </Ctx.Provider>
  );
}

export const useToast = () => useContext(Ctx);

function ToastView({toast, onDone}: {toast: Toast; onDone: () => void}) {
  const m = useMetrics();
  const y = useRef(new Animated.Value(40)).current;
  const o = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    Animated.parallel([Animated.timing(y, {toValue: 0, duration: 220, useNativeDriver: true}), Animated.timing(o, {toValue: 1, duration: 220, useNativeDriver: true})]).start();
    const t = setTimeout(() => Animated.timing(o, {toValue: 0, duration: 220, useNativeDriver: true}).start(onDone), toast.action ? 6500 : 3200);
    return () => clearTimeout(t);
  }, [y, o, onDone, toast.action]);
  const tone = toast.tone === 'error' ? colors.danger : toast.tone === 'success' ? colors.healthy : colors.accentStrong;
  return (
    <Animated.View pointerEvents="box-none" style={{position: 'absolute', left: 0, right: 0, bottom: m.isTV ? 60 : m.isPhone ? 150 : 40, alignItems: 'center', opacity: o, transform: [{translateY: y}]}}>
      <View style={{flexDirection: 'row', alignItems: 'center', gap: m.space.md, backgroundColor: colors.overlay, borderRadius: m.radius.panel, paddingVertical: m.isTV ? 18 : 12, paddingHorizontal: m.isTV ? 28 : 16, borderWidth: 1, borderColor: colors.line, maxWidth: m.isTV ? 900 : m.width - 32, shadowColor: '#000', shadowOpacity: 0.4, shadowRadius: 20, shadowOffset: {width: 0, height: 8}}}>
        <Icon name={toast.icon ?? (toast.tone === 'error' ? 'alert' : 'check')} color={tone} size={m.icon} />
        <Text variant="bodyStrong" style={{flexShrink: 1}}>{toast.message}</Text>
        {toast.action && !m.isTV ? (
          <Focusable onPress={() => { toast.action!.onPress(); onDone(); }} style={({focused}) => ({paddingHorizontal: 12, paddingVertical: 6, borderRadius: 8, backgroundColor: focused ? colors.focus : 'transparent'})}>
            {({focused}) => <Text variant="bodyStrong" color={focused ? colors.accentInk : colors.accentStrong}>{toast.action!.label}</Text>}
          </Focusable>
        ) : null}
      </View>
    </Animated.View>
  );
}
