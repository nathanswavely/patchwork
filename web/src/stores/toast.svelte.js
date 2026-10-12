/**
 * showToast(message, type) is Liquid Textile's toast(): a short message
 * near the foot of the screen, one at a time, read out by a live region.
 * The type is the tone: 'success', 'error', or 'info' (the primary tone).
 * It clears the phone tab bar through --lt-toast-lift (app.css).
 */
import { toast } from '../lib/lt.js';

export function showToast(message, type = 'info') {
  toast(message, { tone: type === 'info' ? 'primary' : type, timeout: 3000 });
}
