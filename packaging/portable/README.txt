Portico Media Server portable package

Unpack the entire archive into a directory where your account can write.
On Linux run ./start-portico.sh. On Windows run start-portico.cmd.
Open http://127.0.0.1:32500/ in your browser.

The launcher finds the web interface and FFmpeg beside the server. Server
state is stored in the "state" directory beside the launcher, regardless of
your working directory. Keep that state directory when updating the
application, and stop the server before copying or replacing state.

To store state elsewhere, set PORTICO_STATE_DIR to an absolute directory
path before running the launcher. PORTICO_WEB_DIR can also be overridden.
Use the same state directory on subsequent starts. Run under an account
with access to your selected media folders; portable launchers do not
install a system service or change firewall rules.
