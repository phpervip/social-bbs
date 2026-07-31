/**
 * @b/shared — THE CONTRACT. Feed Remote imports this exact surface:
 *   import { api, bus, ui } from '@b/shared';
 */
import { bus } from './event-bus';
import api from './api-client';
import { ui } from './ui';

export { api, bus, ui };
export { Avatar, Button, Modal, Toast, Skeleton, EmptyState, ToastHost } from './ui';
export { EVENTS } from './event-bus';
