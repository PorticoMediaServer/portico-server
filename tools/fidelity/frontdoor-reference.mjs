/** Reference-only selectors. Never import into an application or native payload. */
export const frontdoorReferences = {
  phone: {
    selector: '.frame[data-f="39"] > .screen > .col:nth-of-type(1) .phone',
    windowViewport: {width: 1440, height: 1010, deviceScaleFactor: 1},
    viewport: {width: 390, height: 844, deviceScaleFactor: 1},
    anchors: {'primary-action': '.btn.play'},
  },
  androidTV: {
    selector: '.frame[data-f="40"] > .screen.tv',
    windowViewport: {width: 1920, height: 1080, deviceScaleFactor: 1},
    viewport: {width: 1920, height: 1080, deviceScaleFactor: 1},
    anchors: {'primary-action': '.btns .btn.play'},
  },
};

/** App-side public IDs; private selectors/IDs stay exclusively in this tool. */
export const frontdoorStateAdapters = {
  phone: {anchors: {'accounts-welcome-portico': 'primary-action', 'accounts-welcome-portico/label': 'primary-action-label'},
    comparison: {textHandledBySlots: ['primary-action'], actualContentOnly: ['primary-action-label'], contentSlots: [{reference: 'primary-action', actual: 'primary-action-label'}]}},
  androidTV: {anchors: {'tv-front-door-portico': 'primary-action', 'tv-front-door-portico/label': 'primary-action-label'},
    comparison: {focus: {frame: 'primary-action'}, textHandledBySlots: ['primary-action'], actualContentOnly: ['primary-action-label'], contentSlots: [{reference: 'primary-action', actual: 'primary-action-label'}]}},
};
