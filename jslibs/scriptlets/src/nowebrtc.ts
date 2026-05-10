import { createLogger } from './helpers/logger';

const logger = createLogger('nowebrtc');

export function nowebrtc(): void {
  if (!window.RTCPeerConnection) {
    return;
  }

  const pc = (cfg: RTCConfiguration) => {
    logger.log(`document tried to create an RTCPeerConnection with config: ${cfg}`);
  };
  const noop = () => {};
  pc.prototype = {
    close: noop,
    createDataChannel: noop,
    createOffer: noop,
    setRemoteDescription: noop,
    toString: () => '[object RTCPeerConnection]',
  };

  const old = window.RTCPeerConnection;
  window.RTCPeerConnection = pc as unknown as typeof window.RTCPeerConnection;
  if (old.prototype) {
    old.prototype.createDataChannel = () =>
      ({
        close: noop,
        send: noop,
      }) as unknown as RTCDataChannel;
  }
}
