"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const activation_1 = require("./activation");
function activate(context) {
    (0, activation_1.activate)(context);
}
function deactivate() {
    (0, activation_1.deactivate)();
}
