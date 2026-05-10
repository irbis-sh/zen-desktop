import { Engine } from './engine';

export default function (rules: string): void {
  const engine = new Engine(rules);
  engine.start();
}
