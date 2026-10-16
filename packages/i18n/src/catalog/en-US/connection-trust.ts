/**
 * en-US copy for how a device reaches a server: the owner's own certificate, and Apple's rule
 * that plain HTTP stays on the local network. Merged into `enUS` statically.
 */
export const connectionTrustMessages = {
  'connection.certificate.changed': 'This server’s certificate changed, and Portico couldn’t confirm it’s the same server. Trust it only if you know the server was reset or moved.',
  'connection.certificate.unverified': 'This server uses its own certificate and couldn’t prove it’s a Portico server. Trust it only if you know this address.',
  'connection.certificate.untrusted': 'Portico couldn’t check this server’s certificate. Try again.',
  'connection.certificate.trust': 'Trust this server',
  'connection.certificate.question': 'Trust this server’s certificate?',
  'connection.certificate.trusted': 'Server trusted.',
  'connection.certificate.saved': 'Certificates you trusted',
  'connection.certificate.fingerprint': 'Fingerprint {fingerprint}',
  'connection.certificate.forget': 'Forget this certificate?',
  'connection.certificate.forgetBody': 'Portico checks {address}’s certificate again the next time it connects. If the server can’t prove it’s the same server, you’ll be asked first.',
  'connection.certificate.forgetAction': 'Forget',
  'connection.certificate.unsupported': 'If this server uses its own certificate: this TV accepts only certificates from a public authority. Use the server’s http:// address on your home network, or turn on its secure connection.',
  'connection.plainHttp.appleRemote': 'On Apple devices, plain HTTP works only on your home network. Use this server’s https:// address.',
} as const;
